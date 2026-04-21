/*
  [TODO: OrphanBuffer Garbage Collection Specification]

  1. 리소스 고갈 방지 (Memory Safety):
     - 정기적인 찌꺼기 수거:
       - 특정 시간(예: 30분) 이상 부모를 찾지 못한 Vertex는 '유령 데이터'로 간주하고 삭제할 것.
     - LRU (Least Recently Used) 기반 퇴출:
       - Buffer가 꽉 찼을 때(limit 도달), 가장 오래된 고아 Vertex부터 밀어낼 것.

  2. 상태 추적 방어:
     - missingCount에서 제거된 Vertex가 waitingFor의 다른 모든 위치에서도
       완벽히 제거되었는지 확인하는 '참조 무결성' 검사를 주기적으로 수행할 것.

  3. 비상 조치:
     - 특정 Author(노드)가 생성한 고아 Vertex가 과도하게 많을 경우,
       해당 노드의 메시지 수신을 일시적으로 차단(Rate Limiting)하는 로직과 연계할 것.
*/

package core

import (
	"arachnet-bft/types"
	"fmt"
	"sync"
)

type Demeritter interface {
	AddDemerit(author int, amount int, vtx *types.Vertex, reason string)
}

type Orphanage struct {
	mu              sync.Mutex
	lostParents     map[string][]*types.Vertex // lostParent[부모 해시] = [기다리는 자식들]
	lostParentCount map[string]int             // lostParentCount[자식 해시] = 부족한 부모 수
	orphans         map[string]*types.Vertex   // 고아 목록
	capacity        int                        // 최대 잃어버린 부모 수 제한
	slasher         Demeritter
}

func NewOrphanage(limit int, s Demeritter) *Orphanage {
	return &Orphanage{
		lostParents:     make(map[string][]*types.Vertex),
		lostParentCount: make(map[string]int),
		orphans:         make(map[string]*types.Vertex),
		capacity:        limit,
		slasher:         s,
	}
}

// AddOrphan: 부모가 부족한 Vertex (orphan)를 대기실에 등록하네.
func (b *Orphanage) AddOrphan(vtx *types.Vertex, missingParents []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 1. 고아원 용량 초과 확인
	if len(b.lostParentCount) >= b.capacity {
		// 가장 오래된 녀석을 지우거나, 새로운 요청을 거절해야 하네.
		return
	}

	// 2. 고아원 중복 입소 거부: 해시가 같으면 부모목록도 같네.
	if _, exists := b.lostParentCount[vtx.Hash]; exists {
		return
	}

	// 3. 중복 부모 제거 및 등록
	var uniqueHashes []string
	demerit := 0

	// 잃어버린 부모가 적을 때는 Linear (대부분)
	if len(missingParents) < 16 {
		for _, h := range missingParents {
			found := false
			for _, existing := range uniqueHashes {
				if h == existing {
					found = true
					demerit += 1
					break
				}
			}
			if !found {
				uniqueHashes = append(uniqueHashes, h)
				b.lostParents[h] = append(b.lostParents[h], vtx)
			}
		}
	} else {
		// 부모가 많을 때는 map
		tempMap := make(map[string]struct{})
		for _, h := range missingParents {
			if _, ok := tempMap[h]; !ok {
				tempMap[h] = struct{}{}
				uniqueHashes = append(uniqueHashes, h)
				b.lostParents[h] = append(b.lostParents[h], vtx)
			}
		}
		if len(uniqueHashes) != len(missingParents) {
			demerit += 15
		}
	}

	if demerit > 0 && b.slasher != nil {
		fmt.Printf("[ALARM] 노드 %d의 부정 행위 적발! 벌점 %d점 보고하네.\n", vtx.Author, demerit)
		b.slasher.AddDemerit(vtx.Author, demerit, vtx, "duplicate parents detected")
	}
	b.orphans[vtx.Hash] = vtx
	b.lostParentCount[vtx.Hash] = len(uniqueHashes)
}

// OnParentArrival: 부모가 도착했을 때 자식들을 깨워주네.
func (b *Orphanage) OnParentArrival(pHash string) []*types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()

	children, ok := b.lostParents[pHash]
	if !ok {
		return nil
	}

	var readyChildren []*types.Vertex
	for _, child := range children {
		// [방어] 이미 다른 경로로 처리되었을 가능성 체크
		count, exists := b.lostParentCount[child.Hash]
		if !exists {
			continue
		}

		newCount := count - 1
		if newCount <= 0 {
			readyChildren = append(readyChildren, child)
			delete(b.lostParentCount, child.Hash)
			delete(b.orphans, child.Hash)
		} else {
			b.lostParentCount[child.Hash] = newCount
		}
	}

	delete(b.lostParents, pHash)
	return readyChildren
}

func (b *Orphanage) GetOrphan(hash string) *types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orphans[hash]
}

func (o *Orphanage) Size() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.orphans)
}
