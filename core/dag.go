/*
[PROVEN MECHANISMS]
- Deterministic insertion order via sorted orphan release
- Existence-based causality validation (not full validation yet)
- Event-driven orphan resolution (cascade)
- Worklist-based non-recursive insertion
- Modular dependency injection (Config / Fetcher / Orphanage)

[KNOWN LIMITATIONS]
- No full semantic validation (round / equivocation)
- Local non-determinism in orphan eviction (eventual convergence)
- Locking is not strictly fine-grained
- Parent ordering must remain deterministic (critical invariant)
*/

package core

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"fmt"
	"sort"
	"sync"
)

type SyncFetcher interface {
	StartSync(missingHashes []string, suspectID int)
}

type DAG struct {
	mu         sync.RWMutex
	Vertices   map[string]*types.Vertex //vertex hash -> vertex
	RoundIndex map[int][]string
	Buffer     *Orphanage
	Slasher    *Slasher
	Fetcher    SyncFetcher
	Config     *config.Config
}

// NewDAG: DAG와 버퍼를 초기화해서 반환하네.
func NewDAG(fetcher SyncFetcher, cfg *config.Config) *DAG {
	slasher := NewSlasher(cfg)
	orphanage := NewOrphanage(cfg.DAG.OrphanCapacity, slasher)

	return &DAG{
		Vertices:   make(map[string]*types.Vertex),
		RoundIndex: make(map[int][]string),
		Buffer:     orphanage,
		Slasher:    slasher,
		Fetcher:    fetcher,
		Config:     cfg,
	}
}

func (d *DAG) AddVertex(vtx *types.Vertex, currentNodeRound int) {

	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. 중복 체크 (내부 맵 직접 접근으로 데드락 방지)
	if _, exists := d.Vertices[vtx.Hash]; exists {
		return
	}

	// 2. 해시 검증
	calcHash := vtx.CalculateHash()
	if vtx.Hash != calcHash {
		fmt.Printf("[ERROR] DAG: 해시 불일치! 위변조 의심: %s != %s\n", vtx.Hash, calcHash)
		return
	}

	// 3. 없는 부모들 확인
	missing := d.getMissingHashesLocked(vtx.Parents)

	// 4. 부모가 하나라도 없다면 orphanage로!
	if len(missing) > 0 {
		fmt.Printf("[DEBUG] DAG: Vertex %s 고아 보관\n", vtx.Hash[:8])
		d.Buffer.AddOrphan(vtx, missing)

		// Sync: 내 라운드보다 훨씬 높은 녀석이 오면 내가 뒤처진 걸세!
		diff := vtx.Round - currentNodeRound
		if diff > d.Config.DAG.SyncTriggerThreshold {
			d.Fetcher.StartSync(missing, vtx.Author)
		}
		return
	}

	// TODO: validateVertex(vtx)

	// 4. 부모가 다 있다면 정식 삽입!
	d.insertRecursiveLocked(vtx)
}

// insertRecursiveLocked: 락이 걸린 상태에서 연쇄 삽입을 처리하네.
func (d *DAG) insertRecursiveLocked(initialVtx *types.Vertex) {
	worklist := []*types.Vertex{initialVtx}

	for len(worklist) > 0 {
		vtx := worklist[0]
		worklist = worklist[1:]

		if _, exists := d.Vertices[vtx.Hash]; exists {
			continue
		}

		// 결정론적 데이터 기록
		d.Vertices[vtx.Hash] = vtx
		d.RoundIndex[vtx.Round] = append(d.RoundIndex[vtx.Round], vtx.Hash)

		// [결정적 순서 보장] 해시값 기준 오름차순 정렬
		sort.Strings(d.RoundIndex[vtx.Round])
		// TODO: 삽입 시마다 정렬하는 방식은 라운드당 Vertex가 많아질 경우 성능 병목이 될 수 있음.
		// 향후 binary search를 이용한 정렬 삽입(Sorted Insertion)으로 최적화 검토 필요.

		fmt.Printf("[DEBUG] DAG: Vertex %s 삽입 성공 (Round %d)\n", vtx.Hash[:8], vtx.Round)

		// 고아 해방
		readyChildren := d.Buffer.OnParentArrival(vtx.Hash)
		if len(readyChildren) > 0 {
			worklist = append(worklist, readyChildren...)
		}
	}
}

// getMissingHashesLocked: 락을 잡지 않는 내부용 메서드 (데드락 방지용)
func (d *DAG) getMissingHashesLocked(hashes []string) []string {
	var missing []string
	for _, h := range hashes {
		if _, exists := d.Vertices[h]; !exists {
			missing = append(missing, h)
		}
	}
	return missing
}

// GetMissingHashes: 인자로 받은 해시들 중 우리 DAG에 없는 것들만 골라내네.
func (d *DAG) GetMissingHashes(hashes []string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var missing []string
	for _, h := range hashes {
		if _, exists := d.Vertices[h]; !exists {
			missing = append(missing, h)
		}
	}
	return missing
}

// GetVertex: 해시값으로 특정 Vertex를 찾아오네.
func (d *DAG) GetVertex(hash string) *types.Vertex {
	d.mu.RLock() // 읽기 전용 락일세 (성능에 좋지!)
	defer d.mu.RUnlock()
	return d.Vertices[hash]
}

func (d *DAG) insert(initialVtx *types.Vertex) {
	// 1. 앞으로 처리해야 할 Vertex들을 담을 '할 일 목록'이네.
	worklist := []*types.Vertex{initialVtx}

	for len(worklist) > 0 {
		// 목록에서 맨 앞의 녀석을 꺼내옴세.
		vtx := worklist[0]
		worklist = worklist[1:]

		d.mu.Lock()
		// 중복 방지
		if _, exists := d.Vertices[vtx.Hash]; exists {
			d.mu.Unlock()
			continue
		}

		// 2. 진짜 DAG에 기록
		d.Vertices[vtx.Hash] = vtx
		d.RoundIndex[vtx.Round] = append(d.RoundIndex[vtx.Round], vtx.Hash)
		d.mu.Unlock()

		// Log
		fmt.Printf("[DEBUG] DAG: Vertex %s 정식 삽입 성공!\n", vtx.Hash[:8])

		// 3. 연쇄 반응: 이 부모를 기다리던 자식들을 워크리스트에 추가
		readyChildren := d.Buffer.OnParentArrival(vtx.Hash)
		if len(readyChildren) > 0 {
			worklist = append(worklist, readyChildren...)
		}
	}
}

// GetVerticesByRound: 특정 라운드에 생성된 모든 Vertex를 가져오네.
func (d *DAG) GetVerticesByRound(round int) []*types.Vertex {
	d.mu.RLock()
	defer d.mu.RUnlock()

	hashes := d.RoundIndex[round]
	results := make([]*types.Vertex, 0, len(hashes))
	for _, h := range hashes {
		if vtx, exists := d.Vertices[h]; exists {
			results = append(results, vtx)
		}
	}
	return results
}

// GetVotesForVertices: 특정 Vertex들에 대해 수집된 투표(MsgVote)들을 가져오네.
// TODO: 향후 Vote 전용 인덱스나 저장소가 필요할 걸세.
func (d *DAG) GetVotesForVertices(vertices []*types.Vertex) []*types.Message {
	// 지금은 스켈레톤이니 빈 값을 주지만,
	// 나중에 d.VoteIndex[vtx.Hash] 같은 곳에서 꺼내오게 될 걸세.
	return []*types.Message{}
}

// Size: 현재 DAG에 정식으로 삽입된 Vertex의 개수를 반환하네.
func (d *DAG) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.Vertices)
}

// GetTips: 현재 DAG에서 자식이 없는 Vertex들을 반환하네.
func (d *DAG) GetTips() []*types.Vertex {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.Vertices) == 0 {
		// 여기서 제네시스 정점을 생성해서 주거나,
		// 혹은 명시적으로 nil을 주어 상위 계층이 알게 해야 하네.
		return nil
	}

	// 1. 모든 부모 해시를 수집하네.
	hasChild := make(map[string]bool)
	for _, vtx := range d.Vertices {
		for _, parentHash := range vtx.Parents {
			hasChild[parentHash] = true
		}
	}

	// 2. 부모로 한 번도 지목되지 않은 녀석들이 Tips라네!
	var tips []*types.Vertex
	for hash, vtx := range d.Vertices {
		if !hasChild[hash] {
			tips = append(tips, vtx)
		}
	}

	return tips
}
