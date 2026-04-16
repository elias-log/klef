/*
  [TODO: OrphanBuffer Garbage Collection Specification]

  1. 리소스 고갈 방지 (Memory Safety):
     - 정기적인 찌꺼기 수거:
       - 특정 시간(예: 30분) 이상 부모를 찾지 못한 Vertex는 '유령 데이터'로 간주하고 삭제할 것.
     - LRU (Least Recently Used) 기반 퇴출:
       - Buffer가 꽉 찼을 때(limit 도달), 가장 오래된 고아 Vertex부터 밀어낼 것.

  2. 메모리 누수 방지 (Map Cleanup):
     - waitingFor[pHash] 슬라이스에서 특정 Vertex가 제거될 때,
       해당 pHash를 기다리는 자식이 더 이상 없다면 맵에서 키 자체를 삭제할 것.

  3. 상태 추적 방어:
     - missingCount에서 제거된 Vertex가 waitingFor의 다른 모든 위치에서도
       완벽히 제거되었는지 확인하는 '참조 무결성' 검사를 주기적으로 수행할 것.

  4. 비상 조치:
     - 특정 Author(노드)가 생성한 고아 Vertex가 과도하게 많을 경우,
       해당 노드의 메시지 수신을 일시적으로 차단(Rate Limiting)하는 로직과 연계할 것.
*/

package core

import (
	"arachnet-bft/types"
	"sync"
)

type OrphanBuffer struct {
	mu           sync.Mutex
	waitingFor   map[string][]*types.Vertex // 부모 해시 -> 기다리는 자식들
	missingCount map[string]int             // 자식 해시 -> 부족한 부모 수
	capacity     int                        // [보안] 최대 보관 개수 제한
}

func NewOrphanBuffer(limit int) *OrphanBuffer {
	return &OrphanBuffer{
		waitingFor:   make(map[string][]*types.Vertex),
		missingCount: make(map[string]int),
		capacity:     limit,
	}
}

// AddOrphan: 부모가 부족한 Vertex (orphan)를 대기실에 등록하네.
func (b *OrphanBuffer) AddOrphan(vtx *types.Vertex, missingHashes []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 1. [보안] 용량 초과 확인
	if len(b.missingCount) >= b.capacity {
		// 가장 오래된 녀석을 지우거나, 새로운 요청을 거절해야 하네.
		return
	}

	// 2. [논리] 이미 대기 중인 녀석이면 중복 등록 방지
	if _, exists := b.missingCount[vtx.Hash]; exists {
		return
	}

	b.missingCount[vtx.Hash] = len(missingHashes)
	for _, pHash := range missingHashes {
		b.waitingFor[pHash] = append(b.waitingFor[pHash], vtx)
	}
}

// OnParentArrival: 부모가 도착했을 때 자식들을 깨워주네.
func (b *OrphanBuffer) OnParentArrival(pHash string) []*types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()

	children, ok := b.waitingFor[pHash]
	if !ok {
		return nil
	}

	var readyChildren []*types.Vertex
	for _, child := range children {
		// [방어] 이미 다른 경로로 처리되었을 가능성 체크
		count, exists := b.missingCount[child.Hash]
		if !exists {
			continue
		}

		newCount := count - 1
		if newCount <= 0 {
			readyChildren = append(readyChildren, child)
			delete(b.missingCount, child.Hash)
		} else {
			b.missingCount[child.Hash] = newCount
		}
	}

	delete(b.waitingFor, pHash)
	return readyChildren
}
