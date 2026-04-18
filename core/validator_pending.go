package core

import (
	"arachnet-bft/types"
	"container/heap"
	"math/rand"
	"time"
)

// Item: 큐에서 관리할 요청 정보
type PendingItem struct {
	Hash        string
	RequestTime time.Time // Monotonic Clock차이 해결용. 맵에 저장된 시간과 1:1 매칭할 녀석일세.
	ExpiryTime  time.Time
	index       int // heap 내부 관리를 위한 인덱스
}

type PendingMeta struct {
	RequestTime time.Time
	ExpiryTime  time.Time
	RetryCount  int
}

// IsRequestPending: [TODO] 요청 장부(Pending Map)를 확인하는 로직이 들어갈 자리일세.
// 나중에 pendingRequests 맵을 도입하면 여기서 실제로 확인하게 될 걸세.
func (v *Validator) IsRequestPending(hash string) bool {
	v.pendingMu.RLock()
	defer v.pendingMu.RUnlock() // 함수가 끝날 때까지 안전하게 보호하세.

	meta, exists := v.pendingRequests[hash]
	if !exists {
		return false
	}

	// 만료 여부만 판단해서 알려주게나.
	// 별도의 고루틴 startPendingCleanup()으로 처리해서 리드만 해도 되게 처리하겠네.
	return time.Now().Before(meta.ExpiryTime)
}

// Validator 시작 시 한 번 띄워두는 녀석일세.
func (v *Validator) startPendingCleanup() {
	// 1. Ticker Leak 방지: 안해두면 죽은 애가 RequestTimeout의 최대 2배까지 살아남네.
	checkInterval := v.Config.Request.BaseTimeout / 2
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			v.pendingMu.Lock()
			now := time.Now()

			// 2. Priority Queue 활용: 만료된 녀석들만 쏙쏙 골라내기
			// 맵 전체 순회(O(N)) 없이 큐의 앞부분(O(log N))만 확인하네.
			for v.pendingQueue.Len() > 0 {
				// 가장 오래된 요청 확인
				item := v.pendingQueue[0]
				meta, exists := v.pendingRequests[item.Hash] // 이제 meta는 PendingMeta 타입일세!

				// 맵에 저장된 시간과 큐에 들어있던 생성 시간이 다르면 '가짜' 혹은 '낡은' 데이터일세.
				if !exists || !meta.RequestTime.Equal(item.RequestTime) {
					heap.Pop(&v.pendingQueue)
					continue
				}

				// 제일 오래된 놈도 아직 안 죽었으면 나머지도 다 살아있네!
				if now.Before(item.ExpiryTime) {
					break
				}

				// 만료된 녀석 제거
				heap.Pop(&v.pendingQueue)
				delete(v.pendingRequests, item.Hash)
			}
			v.pendingMu.Unlock()

		case <-v.ctx.Done():
			// 3. Validator 종료 시 고루틴 종료
			return
		}
	}
}

func (v *Validator) AddPendingRequest(hash string) {
	v.pendingMu.Lock()
	defer v.pendingMu.Unlock()

	now := time.Now()
	meta, exists := v.pendingRequests[hash]

	// 2. [중복 체크] 이미 유효한 요청이 진행 중이면 무시
	if exists && now.Before(meta.ExpiryTime) {
		return
	}

	// 3. 리트라이 횟수 계산
	retryCount := 0
	if exists {
		retryCount = meta.RetryCount + 1
	}

	// 4. 지수 백오프 및 지터 계산
	// backoff overflow 방지 / MaxBackoff 적용
	backoff := v.Config.Request.BaseTimeout * time.Duration(1<<retryCount)
	if backoff > v.Config.Request.MaxBackoff {
		backoff = v.Config.Request.MaxBackoff
	}

	// Jitter 계산: 0 ~ backoff/5 사이의 무작위 값
	jitter := time.Duration(0)
	if backoff > 0 {
		jitter = time.Duration(rand.Int63n(int64(backoff / 5)))
	}

	expiry := now.Add(backoff + jitter)

	// 5. map(진실) 갱신 & Queue() 삽입
	v.pendingRequests[hash] = PendingMeta{
		RequestTime: now,
		ExpiryTime:  expiry,
		RetryCount:  retryCount,
	}

	heap.Push(&v.pendingQueue, &PendingItem{
		Hash:        hash,
		RequestTime: now,
		ExpiryTime:  expiry,
	})
}

// GetReadyQuorum: 어떤 라운드든 2f+1이 모인 게 있다면 (투표들, 그 라운드)를 반환하네.
func (v *Validator) GetReadyQuorum() ([]*types.Message, int) {
	// 1. 현재 라운드 혹은 이전 라운드부터 뒤져보세 (보통 현재 라운드)
	round := v.Round
	votes := v.votePool.GetVotesByRound(round)

	policy := v.GetQuorumPolicy()
	requiredQuorum := policy.GetQuorumSize(types.QCGlobal)

	if len(votes) >= requiredQuorum {
		return votes, round
	}

	// 만약 현재 라운드가 아니더라도, 혹시나 쿼럼이 형성된 라운드가 있는지 체크할 수도 있네.
	return nil, 0
}

// PriorityQueue
type PriorityQueue []*PendingItem

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].ExpiryTime.Before(pq[j].ExpiryTime) }
func (pq PriorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i]; pq[i].index = i; pq[j].index = j }
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PendingItem)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1
	*pq = old[0 : n-1]
	return item
}
