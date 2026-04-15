package core

import (
	"arachnet-bft/types"
	"sync"
)

type OrphanBuffer struct {
	mu           sync.Mutex
	waitingFor   map[string][]*types.Vertex // 부모 해시 -> 기다리는 자식들
	missingCount map[string]int             // 자식 해시 -> 부족한 부모 수
}

func NewOrphanBuffer() *OrphanBuffer {
	return &OrphanBuffer{
		waitingFor:   make(map[string][]*types.Vertex),
		missingCount: make(map[string]int),
	}
}

// AddOrphan: 부모가 부족한 Vertex (orphan)를 대기실에 등록하네.
func (b *OrphanBuffer) AddOrphan(vtx *types.Vertex, missingHashes []string) {
	b.mu.Lock()
	defer b.mu.Unlock() //defer는 "함수의 닫는 중괄호때 하겠다"는 예약

	b.missingCount[vtx.Hash] = len(missingHashes)
	for _, pHash := range missingHashes {
		b.waitingFor[pHash] = append(b.waitingFor[pHash], vtx)
	}
}

// OnParentArrival: 부모가 도착했을 때 자식들을 깨워주네.
func (b *OrphanBuffer) OnParentArrival(pHash string) []*types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()

	var readyChildren []*types.Vertex
	// 이 부모를 기다리던 자식들을 찾아서
	if children, ok := b.waitingFor[pHash]; ok {
		for _, child := range children {
			b.missingCount[child.Hash]--
			// 모든 부모가 다 왔다면 대기 목록에서 제외하고 반환!
			if b.missingCount[child.Hash] == 0 {
				readyChildren = append(readyChildren, child)
				delete(b.missingCount, child.Hash)
			}
		}
		// 이제 이 부모를 기다리는 자식은 없으니 맵에서 제거하네.
		delete(b.waitingFor, pHash)
	}
	return readyChildren
}
