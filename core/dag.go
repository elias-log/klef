//Vertex 저장소 및 그래프 관계 관리
/*
  IMPLEMENTED:
  아래 기능들은 검증 및 구현이 완료되어 시스템의 중추 역할을 수행 중이네.

  1. 동적 의존성 주입 (Dependency Injection):
     - Config(장부), Fetcher(동기화 장치), OrphanBuffer(미아 보호소)를
       생성 시점에 주입받아 모듈 간 결합도를 낮추고 유연성을 확보함.

  2. 지능형 Vertex 삽입 로직 (AddVertex):
     - 진위 확인: Vertex의 해시를 직접 재계산하여 데이터 무결성을 검증함.
     - 의존성 검사: 부모 노드의 존재 여부를 파악하여 즉시 삽입 혹은 대기실 행을 결정함.
     - 능동적 동기화: 현재 노드와 제안된 Vertex 간의 라운드 격차(SyncTriggerThreshold)를
       분석하여 필요시 자동으로 Fetcher를 가동함.

  3. 안전한 반복적 삽입 알고리즘 (Iterative Insertion):
     - 재귀 호출(Recursion)을 배제하고 워크리스트(Worklist) 기반의 반복문을 채택함.
     - 스택 오버플로를 방지하고, 대규모 도미노 삽입 상황에서도 메모리 안정성을 보장함.
     - 세밀한 락(Lock) 제어를 통해 데이터 일관성과 성능의 균형을 맞춤.

  4. 고아 노드 관리 (Orphan Handling):
     - 부모가 없는 노드를 OrphanBuffer에 격리하고, 부모 도착 시 즉시 해방시키는
       이벤트 기반의 연쇄 삽입 시스템 구축 완료.

  5. 설정 기반 운영 (Policy-Mechanism Separation):
     - 하드코딩된 수치(2, 10000 등)를 Config 객체로 이관함.
     - OrphanCapacity, SyncTriggerThreshold 등의 세련된 명칭을 사용하여
       코드의 가독성과 운영 편의성을 극대화함.
*/

/*
	TODO:
	1. 인과 관계 및 정당성 검증 (Validity Check)
	- 부모 라운드 검증: vtx.Round가 모든 부모 라운드보다 정확히 1 큰지 확인 (라운드 점프 방지)
	- 작성자 중복 투표(Equivocation) 탐지: 동일한 Author가 동일한 Round에 서로 다른 두 개의 Vertex를 생성했는지 감시하고, 발견 시 Slashing에 전달
	- 부모 참조 무결성: 부모 해시들이 실제로 유효한 구조를 가졌는지, 자기 자신을 부모로 참조하는 순환 참조는 없는지 체크

	2.합의 알고리즘 인터페이스 (Consensus Integration)
	- Anchor 선출: 특정 라운드에서 합의의 기준이 될 Anchor Vertex를 결정하는 로직을 구현
	- Virtual Voting (가상 투표): 직접 메시지를 주고받지 않고, DAG의 연결 구조만으로 투표 결과를 계산하는 Order 함수가 필요
	- Finality 결정: 어떤 Vertex가 "절대 뒤집히지 않는다"고 확정(Finalized)되는 순간을 정의하고, 이를 통해 트랜잭션의 실행 순서를 확정

	3. 성능 및 자원 관리 (Maintenance)
	- DAG Pruning: 너무 오래된 라운드의 데이터는 메모리에서 해제하고 스토리지(DB)로 옮기거나 삭제하는 로직이 필요
	- Snapshoting: 특정 시점의 DAG 상태를 요약하여 새로운 노드가 빠르게 동기화할 수 있도록 돕는 스냅샷 기능을 고려
	- Index 최적화: 현재 RoundIndex 외에 AuthorIndex 등을 추가하여 특정 노드가 만든 Vertex를 빠르게 조회할 수 있게 확장

	4. 동기화 고도화 (Sync Expansion)
	- Batch Fetching: GetMissingHashes 결과가 너무 많을 경우, 이를 적절한 크기로 쪼개서 여러 피어에게 분산 요청하는 로직을 Fetcher와 연계
	- Priority Queue: worklist를 단순 슬라이스가 아닌 라운드 순 정렬 큐로 바꿔서, 낮은 라운드부터 차례대로 안정적으로 삽입되게 보장
*/

package core

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"fmt"
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

	// 1. 중복 체크
	if d.GetVertex(vtx.Hash) != nil {
		return
	}

	// 2. 해시 검증
	calculatedHash := vtx.CalculateHash()
	if vtx.Hash != calculatedHash {
		fmt.Printf("[ERROR] DAG: 해시 불일치! 위변조 의심: %s != %s\n", vtx.Hash, calculatedHash)
		return
	}

	// 3. 없는 부모들 확인
	missing := d.GetMissingHashes(vtx.Parents)

	// 3. 부모가 하나라도 없다면 Buffer로!
	if len(missing) > 0 {
		//Log
		fmt.Printf("[DEBUG] DAG: Vertex %s 는 고아일세. 누락된 부모: %v\n", vtx.Hash[:8], missing)

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
	d.insert(vtx)
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
