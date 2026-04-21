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

// LOCK ORDER GUARANTEE:
// DAG.mu must always be acquired before Orphanage.mu.
// Orphanage must NEVER call back into DAG.

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

// Lock Hierarchy:
// DAG.mu  >  Orphanage.mu
func (d *DAG) AddVertex(vtx *types.Vertex, currentNodeRound int) {

	// 1. 원본 기준 해시 검증
	calcHash := vtx.CalculateHash()
	if vtx.Hash != calcHash {
		fmt.Printf("[ERROR] DAG: 해시 불일치!: %s\n", vtx.Hash)
		// TODO: reject and slash (단순 전송오류일수도 있나? 검토 필요)
		return
	}

	// 2. 원본을 정렬하고 중복이 제거된 상태로 변경
	if types.IsMalformed(vtx.Parents) {
		fmt.Printf("[CRITICAL] 비잔틴 노드 %d 확인: 해시가 맞지만, 형식오류데이터 고의 전송\n", vtx.Author)
		d.Slasher.AddDemerit(
			vtx.Author,
			d.Config.Security.MalformedVertexPenalty,
			vtx,
			"Malformed Parents: Unsorted or Duplicated",
		)
		return
	}

	// 3. 중복 및 부모 확인
	d.mu.RLock()
	_, exists := d.Vertices[vtx.Hash]
	missing := d.getMissingHashesLocked(vtx.Parents)
	d.mu.RUnlock()
	if exists {
		return
	}

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

	// 5. 정식 삽입
	d.processInsertion(vtx)
}

// processInsertion:
func (d *DAG) processInsertion(initialVtx *types.Vertex) {
	worklist := []*types.Vertex{initialVtx}

	for len(worklist) > 0 {
		vtx := worklist[0]
		worklist = worklist[1:]

		d.mu.Lock()
		if _, exists := d.Vertices[vtx.Hash]; exists {
			d.mu.Unlock()
			continue
		}

		// 데이터 기록
		d.Vertices[vtx.Hash] = vtx
		d.RoundIndex[vtx.Round] = append(d.RoundIndex[vtx.Round], vtx.Hash)

		// [결정적 순서 보장] 해시값 기준 오름차순 정렬
		sort.Strings(d.RoundIndex[vtx.Round])
		// TODO Phase1: 삽입 시마다 정렬하는 방식은 라운드당 Vertex가 많아질 경우 성능 병목이 될 수 있음.
		// 향후 binary search를 이용한 Sorted Insertion으로 최적화 검토 필요.

		// 고아 해방: Orphanage 내부의 Lock은 DAG Lock의 하위 계급이므로 안전하네.
		readyChildren := d.Buffer.OnParentArrival(vtx.Hash)
		d.mu.Unlock()

		fmt.Printf("[DEBUG] DAG: Vertex %s 삽입 성공 (Round %d)\n", vtx.Hash[:8], vtx.Round)
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
