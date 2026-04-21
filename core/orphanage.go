/*
[Orphanage Design Principles & Invariants]

1. Invariants (원칙):
    - Internal Consistency: lostParentCount의 key set은 orphans의 key set과 항상 일치해야 하네.
    - Zero-Count Liberation: lostParentCount가 0이 되는 즉시 해당 Vertex는 orphanage를 떠나
    	readyChildren으로 이동해야 하네.
    - Deterministic Integration: OnParentArrival을 통해 해방되는 Vertex들의 순서는
    	네트워크 도착 순서와 무관하게 반드시 Hash 기반으로 정렬되어 DAG에 삽입되어야 하네.

2. Limitations & Local Determinism (한계 및 로컬 결정성):
    - Localized Eviction: findVictim은 'missingCount'라는 로컬 관측 상태에 의존하네.
       따라서 노드마다 부모를 인지하는 시점이 다를 경우, 동일한 Capacity 상황에서도
       축출되는 대상이 노드 간에 일치하지 않을 수 있네.
    - Convergence Principle: 축출 대상의 비결정성은 Transient한 현상이네.
       결국 유실된 부모 데이터가 재수신되면 모든 노드는 동일한 최종 DAG 구조로 converge하네.
    - Trade-off: 'Total Parent Count' 기반의 전역 결정성보다 'Missing Count' 기반의
       orphanage 회전율(Efficiency)을 우선순위로 채택했네.

- Convergence Guarantee (Assumption):
	The convergence to an identical DAG is guaranteed under the assumption of eventual data delivery
	(i.e., all missing parent vertices are eventually received) and no permanent data loss.

- Non-Consensus Scope:
	Orphanage itself is not consensus-critical,
	but its output must be deterministically consumed.
*/

package core

import (
	"arachnet-bft/types"
	"sort"
	"sync"
)

type Orphanage struct {
	mu              sync.Mutex
	lostParents     map[string][]*types.Vertex // lostParent[부모 해시] = [기다리는 자식들]
	lostParentCount map[string]int             // lostParentCount[자식 해시] = 부족한 부모 수
	orphans         map[string]*types.Vertex   // 고아 목록
	capacity        int                        // 최대 잃어버린 부모 수 제한
}

func NewOrphanage(limit int) *Orphanage {
	return &Orphanage{
		lostParents:     make(map[string][]*types.Vertex),
		lostParentCount: make(map[string]int),
		orphans:         make(map[string]*types.Vertex),
		capacity:        limit,
	}
}

// AddOrphan: 부모가 부족한 Vertex를 등록하되, 용량 초과 시 결정론적으로 축출하네.
func (b *Orphanage) AddOrphan(vtx *types.Vertex, missingParents []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 1. 고아원 중복 입소 거부: 해시가 같으면 부모목록도 같네.
	if _, exists := b.lostParentCount[vtx.Hash]; exists {
		return
	}

	// 2. 고아원 용량 초과 확인
	missingCount := len(missingParents)
	if len(b.lostParentCount) >= b.capacity {
		victim := b.findVictim(vtx.Hash, missingCount)

		// 만약 새로 들어온 녀석이 희생양이라면, 입소시키지 않고 바로 종료하네.
		if victim == vtx.Hash {
			return
		}

		// 기존에 있던 녀석이 희생양이라면, 그 녀석을 쫓아내고 자리를 만드네.
		b.removeOrphan(victim)
	}

	// 3. 관계 등록 (이미 missingParents는 Unique & Sorted임이 보장됨)
	for _, h := range missingParents {
		b.lostParents[h] = append(b.lostParents[h], vtx)
	}

	b.orphans[vtx.Hash] = vtx
	b.lostParentCount[vtx.Hash] = missingCount
}

// findVictim: 고아원 수용량을 위해 희생될 Vertex의 해시를 결정론적으로 선택하네.
func (b *Orphanage) findVictim(candidateHash string, candidateMissingCount int) string {
	// 처음에는 새로 들어오려는 녀석을 희생양 후보로 잡네.
	victimHash := candidateHash
	maxMissing := candidateMissingCount

	for h, count := range b.lostParentCount {
		// 우선순위 1: 기다려야 하는 부모 수가 더 많은 녀석을 선택
		if count > maxMissing {
			victimHash = h
			maxMissing = count
		} else if count == maxMissing {
			// 우선순위 2: 부모 수가 같다면 해시값이 더 큰(사전순 뒤쪽) 녀석을 선택
			// 이는 모든 노드가 동일한 대상을 지목하게 하는 결정론적 기준일세.
			if h > victimHash {
				victimHash = h
			}
		}
	}
	return victimHash
}

// OnParentArrival: 부모가 도착했을 때 결정론적 순서로 자식들을 깨워주네.
func (b *Orphanage) OnParentArrival(pHash string) []*types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()

	children, ok := b.lostParents[pHash]
	if !ok {
		return nil
	}

	// 1. [Fix 1] 대기 중인 자식들 자체를 해시 순으로 정렬하세.
	// 이렇게 하면 어느 노드에서든 부모를 기다리는 명단이 동일해지지.
	sort.Slice(children, func(i, j int) bool {
		return children[i].Hash < children[j].Hash
	})

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

	// 2. [Fix 2] 부모 찾은 자식들도 한 번 더 정렬하세.
	// 도미노처럼 연쇄 삽입될 때 이 순서가 곧 DAG.insert의 순서가 되니까!
	sort.Slice(readyChildren, func(i, j int) bool {
		return readyChildren[i].Hash < readyChildren[j].Hash
	})

	delete(b.lostParents, pHash)
	return readyChildren
}

// removeOrphan: 특정 고아를 모든 인덱스에서 제거하네.
func (b *Orphanage) removeOrphan(hash string) {
	vtx, exists := b.orphans[hash]
	if !exists {
		return
	}

	// 1. 부모 인덱스(lostParents)에서 자식 명단 삭제
	for _, pHash := range vtx.Parents {
		if list, ok := b.lostParents[pHash]; ok {
			// 해당 리스트에서 본인만 쏙 빼내기
			for i, child := range list {
				if child.Hash == hash {
					b.lostParents[pHash] = append(list[:i], list[i+1:]...)
					break
				}
			}
			// 만약 이 부모를 기다리는 자식이 더 이상 없다면 맵에서 삭제
			if len(b.lostParents[pHash]) == 0 {
				delete(b.lostParents, pHash)
			}
		}
	}

	// 2. 카운트 및 객체 보관소에서 삭제
	delete(b.lostParentCount, hash)
	delete(b.orphans, hash)
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
