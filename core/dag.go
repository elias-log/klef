//Vertex 저장소 및 그래프 관계 관리

package core

import (
	"arachnet-bft/types"
	"sync"
)

type DAG struct {
	mu sync.RWMutex

	Vertices   map[string]*types.Vertex //vertex hash -> vertex
	RoundIndex map[int][]string
	Buffer     *OrphanBuffer
}

// NewDAG: DAG와 버퍼를 초기화해서 반환하네.
func NewDAG() *DAG {
	return &DAG{
		Vertices:   make(map[string]*types.Vertex),
		RoundIndex: make(map[int][]string),
		Buffer:     NewOrphanBuffer(), // OrphanBuffer도 새로 만들어야 하네.
	}
}

func (d *DAG) AddVertex(vtx *types.Vertex, currentNodeRound int) {
	// 1. 없는 부모들을 담을 리스트(Slice) 준비
	var missingHashes []string

	d.mu.RLock() // 읽기 락을 걸고 확인하세
	for _, pHash := range vtx.Parents {
		if d.Vertices[pHash] == nil {
			missingHashes = append(missingHashes, pHash)
		}
	}
	d.mu.RUnlock()

	// 2. 부모가 하나라도 없다면?
	if len(missingHashes) > 0 {
		// 내부 맵을 직접 건드리지 말고, 만들어둔 함수에 맡기게!
		d.Buffer.AddOrphan(vtx, missingHashes)

		// 내 라운드와 이 Vertex의 라운드 차이가 2초과면 즉시 Fetcher 가동!
		if currentNodeRound-vtx.Round > 2 {
			// d.Fetcher.Request(missingHashes) 호출!
		}
		return
	}

	// 3. 부모가 다 있다면 정식 삽입!
	d.insert(vtx)
}

// GetVertex: 해시값으로 특정 Vertex를 찾아오네.
func (d *DAG) GetVertex(hash string) *types.Vertex {
	d.mu.RLock() // 읽기 전용 락일세 (성능에 좋지!)
	defer d.mu.RUnlock()
	return d.Vertices[hash]
}

func (d *DAG) insert(vtx *types.Vertex) {
	d.mu.Lock()

	// 이미 있는 녀석이면 무시 (네트워크 중복 메시지 방지)
	if _, exists := d.Vertices[vtx.Hash]; exists {
		d.mu.Unlock()
		return
	}

	// 1. 진짜 DAG에 기록
	d.Vertices[vtx.Hash] = vtx
	d.RoundIndex[vtx.Round] = append(d.RoundIndex[vtx.Round], vtx.Hash)
	d.mu.Unlock()

	// 2. 대기실(Buffer)에 "야, 니네 부모 왔다!"라고 말해주네.
	readyChildren := d.Buffer.OnParentArrival(vtx.Hash)

	// 3. 해방된 자식들도 차례대로 DAG에 삽입 (재귀적 도미노)
	for _, child := range readyChildren {
		d.insert(child)
	}
}

func (d *DAG) StillMissing(hashes []string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, h := range hashes {
		if _, exists := d.Vertices[h]; !exists {
			return true // 하나라도 없으면 여전히 가져와야 하는 상태지
		}
	}
	return false
}
