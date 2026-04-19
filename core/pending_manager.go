package core

import (
	"arachnet-bft/config"
	"arachnet-bft/internal/ds"
	"container/heap"
	"context"
	"math/rand"
	"sync"
	"time"
)

const (
	minCleanupInterval = 100 * time.Millisecond
	maxCleanupInterval = 500 * time.Millisecond
)

// PendingItem: Priority Queue 내부에서 관리될 데이터
type PendingItem struct {
	Hash        string
	RequestTime time.Time
	ExpiryTime  time.Time
	index       int
}

// PendingMeta: 맵에서 관리될 상세 정보
type PendingMeta struct {
	RequestTime time.Time
	ExpiryTime  time.Time
	RetryCount  int
}

// PendingManager: fetch 요청목록 관리자
type PendingManager struct {
	mu              sync.RWMutex
	pendingRequests map[string]PendingMeta
	pendingQueue    ds.PriorityQueue
	cfg             *config.Config
	rng             *rand.Rand
}

// NewPendingManager 생성자
func NewPendingManager(cfg *config.Config) *PendingManager {
	return &PendingManager{
		pendingRequests: make(map[string]PendingMeta),
		pendingQueue:    make(ds.PriorityQueue, 0),
		cfg:             cfg,
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Add: 새로운 요청을 등록하거나 기존 요청의 재시도 시간을 갱신하네.
func (pm *PendingManager) Add(hash string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	meta, exists := pm.pendingRequests[hash]

	// 1. [중복 체크] 이미 유효한 요청이 진행 중이면 무시
	if exists && now.Before(meta.ExpiryTime) {
		return
	}

	// 2. 리트라이 횟수 계산
	retryCount := 0
	if exists {
		retryCount = meta.RetryCount + 1
	}

	// [Updated] 오버플로우 방지
	if retryCount > 15 {
		retryCount = 15
	}

	// 3. 지수 백오프 및 지터 계산
	backoff := pm.cfg.Request.BaseTimeout * time.Duration(1<<retryCount)
	if backoff > pm.cfg.Request.MaxBackoff {
		backoff = pm.cfg.Request.MaxBackoff
	}

	// [Updated] 로컬 RNG로 지터 계산: Global race 제거
	jitter := time.Duration(0)
	if backoff > 0 {
		jitter = time.Duration(pm.rng.Int63n(int64(backoff / 5)))
	}

	expiry := now.Add(backoff + jitter)

	// 4. map 갱신 & Queue 삽입
	pm.pendingRequests[hash] = PendingMeta{
		RequestTime: now,
		ExpiryTime:  expiry,
		RetryCount:  retryCount,
	}

	heap.Push(&pm.pendingQueue, &PendingItem{
		Hash:        hash,
		RequestTime: now,
		ExpiryTime:  expiry,
	})
}

// Remove: 데이터가 성공적으로 수신되었을 때, 즉시 장부에서 제거하네.
func (pm *PendingManager) Remove(hash string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 맵에서 지워버리면, 나중에 cleanup 루프가 큐를 돌다가
	// !exists 조건에 걸려 큐에서도 자연스럽게 빠지게 될 걸세.
	delete(pm.pendingRequests, hash)

	// 참고: 여기서 큐(Heap)를 직접 뒤져서 삭제하는 건 O(N)이라 성능에 좋지 않네.
	// cleanup에 구현해둔 Lazy Deletion을 사용하고 맵만 지우도록 하겠네!
}

// IsPending: 특정 해시가 현재 처리 대기 중인지 확인하네.
func (pm *PendingManager) IsPending(hash string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	meta, exists := pm.pendingRequests[hash]
	return exists && time.Now().Before(meta.ExpiryTime)
}

// StartCleanupLoop: 만료된 요청을 주기적으로 정리하네.
func (pm *PendingManager) StartCleanupLoop(ctx context.Context) {
	interval := pm.getCleanupInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.cleanup()
		case <-ctx.Done():
			return
		}
	}
}

func (pm *PendingManager) getCleanupInterval() time.Duration {
	interval := pm.cfg.Request.BaseTimeout / 4

	if interval < minCleanupInterval {
		return minCleanupInterval
	}
	if interval > maxCleanupInterval {
		return maxCleanupInterval
	}
	return interval
}

func (pm *PendingManager) cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	for pm.pendingQueue.Len() > 0 {
		// 가장 오래된 요청 확인
		item := pm.pendingQueue[0].(*PendingItem)
		meta, exists := pm.pendingRequests[item.Hash]

		// [Updated] 데이터 무결성 체크 (RequestTime, ExpiryTime 동시 검증)
		if !exists || !meta.RequestTime.Equal(item.RequestTime) || !meta.ExpiryTime.Equal(item.ExpiryTime) {
			heap.Pop(&pm.pendingQueue)
			continue
		}

		// 제일 오래된 놈도 아직 안 죽었으면 나머지도 다 살아있네!
		if now.Before(item.ExpiryTime) {
			break
		}

		// 만료된 녀석 제거
		heap.Pop(&pm.pendingQueue)
		delete(pm.pendingRequests, item.Hash)
	}
}

//Less: 우선순위 결정 (만료 시간이 빠를수록 앞으로!)
func (pi *PendingItem) Less(other ds.HeapItem) bool {
	return pi.ExpiryTime.Before(other.(*PendingItem).ExpiryTime)
}

//SetIndex: ds.priority_queue가 idx를 알려줄 때 기록하네.
func (pi *PendingItem) SetIndex(idx int) { pi.index = idx }

//GetIndex: 현재 index를 반환하네.
func (pi *PendingItem) GetIndex() int { return pi.index }
