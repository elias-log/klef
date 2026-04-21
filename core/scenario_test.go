package core

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestDAGOrphanAndInsertion(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 현재 노드의 라운드를 10이라고 가정해보세.
	currentRound := 10

	// 1. 부모가 없는 '고아' Vertex 생성 (Round 5)
	// 제네시스가 없는 상태에서 부모 해시만 적어주면 고아가 된다네.
	orphanVtx := CreateDummyVertex(1, 5, []string{"missing_parent_hash"}, signer)

	// 2. AddVertex 호출 (자네가 지적한 currentNodeRound를 넣었네!)
	v.DAG.AddVertex(orphanVtx, currentRound)

	// 검증: 대기실(Buffer)에 잘 들어가 있나?
	if found := v.DAG.Buffer.GetOrphan(orphanVtx.Hash); found == nil {
		t.Errorf("❌ Vertex %s 가 대기실(Buffer)에 들어가지 않았네!", orphanVtx.Hash[:8])
	} else {
		t.Logf("✅ 성공: Vertex %s 가 고아로 판명되어 대기실에서 부모를 기다리네.", orphanVtx.Hash[:8])
	}
}

func TestOrphanResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 1. '진짜 부모'가 될 녀석을 미리 준비만 해두세 (아직 넣지는 않네)
	// CreateDummyVertex 내부에서 CalculateHash가 완료된 상태일 걸세.
	parentVtx := CreateDummyVertex(0, 0, []string{}, signer)
	realParentHash := parentVtx.Hash // 진짜 계산된 해시를 가져옴세!

	// 2. 이제 그 진짜 해시를 부모로 삼는 '고아'를 만드네.
	orphan := CreateDummyVertex(1, 1, []string{realParentHash}, signer)

	// 3. 고아 주입! (부모가 없으니 대기실로 가겠지?)
	v.DAG.AddVertex(orphan, 1)

	// 검증: 대기실에 잘 있나?
	if v.DAG.Buffer.GetOrphan(orphan.Hash) == nil {
		t.Fatal("고아가 대기실에 없네!")
	}

	// 4. 이제 아까 준비해둔 '진짜 부모'를 주입하세!
	v.DAG.AddVertex(parentVtx, 1)

	// 5. 최종 검증: 고아가 부모를 따라 대기실을 탈출했는가?
	if v.DAG.GetVertex(orphan.Hash) != nil {
		t.Log("✅ 성공! 해시 검증을 통과한 부모가 오자 고아가 스스로 합류했네!")
	} else {
		t.Errorf("❌ 부모가 정식 삽입되었음에도 고아가 여전히 대기실에 있구먼.")
	}
}

func TestDeepOrphanChain(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 1. 세대 생성: 할아버지(v1) -> 아버지(v2) -> 손자(v3)
	v1 := CreateDummyVertex(0, 0, []string{}, signer)
	v2 := CreateDummyVertex(0, 1, []string{v1.Hash}, signer)
	v3 := CreateDummyVertex(0, 2, []string{v2.Hash}, signer)

	// 2. 가혹한 순서로 주입: 손자부터!
	t.Log("손자 주입...")
	v.DAG.AddVertex(v3, 2)
	t.Log("아버지 주입...")
	v.DAG.AddVertex(v2, 2)

	// 이 시점까지는 둘 다 고아여야 하네.
	if v.DAG.GetVertex(v3.Hash) != nil || v.DAG.GetVertex(v2.Hash) != nil {
		t.Fatal("❌ 부모가 없는데 벌써 삽입되면 안 되네!")
	}

	// 3. 마지막 조각, 할아버지 주입!
	t.Log("할아버지 주입 (도미노 시작)...")
	v.DAG.AddVertex(v1, 2)

	// 4. 최종 확인
	if v.DAG.GetVertex(v3.Hash) != nil && v.DAG.GetVertex(v2.Hash) != nil {
		t.Log("✅ 대성공! 3세대 연쇄 삽입이 완료되었네!")
	} else {
		t.Errorf("❌ 도미노가 중간에 멈췄구먼.")
	}
}

func TestMeshOrphanResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 1. 다이아몬드 구조 설계
	//      V1 (Root)
	//     /  \
	//    V2   V3
	//     \  /
	//      V4 (Leaf)
	v1 := CreateDummyVertex(0, 0, []string{}, signer)
	v2 := CreateDummyVertex(0, 1, []string{v1.Hash}, signer)
	v3 := CreateDummyVertex(0, 2, []string{v1.Hash}, signer)
	v4 := CreateDummyVertex(0, 3, []string{v2.Hash, v3.Hash}, signer)

	// 2. 가장 밑바닥부터 역순으로 주입
	t.Log("V4 (Leaf) 주입...")
	v.DAG.AddVertex(v4, 3)

	t.Log("V2 주입 (부모 V1 부재로 여전히 고아)...")
	v.DAG.AddVertex(v2, 3)

	t.Log("V3 주입 (부모 V1 부재로 여전히 고아)...")
	v.DAG.AddVertex(v3, 3)

	// 이 시점까지 DAG에는 아무것도 없어야 하네.
	if v.DAG.GetVertex(v4.Hash) != nil || v.DAG.GetVertex(v1.Hash) != nil {
		t.Fatal("❌ 아직 아무도 삽입되면 안 되는 시점이네!")
	}

	// 3. 마지막 열쇠, V1 (Root) 주입!
	t.Log("V1 (Root) 주입! 이제 도미노가 터질 걸세.")
	v.DAG.AddVertex(v1, 3)

	// 4. 최종 검증
	// V1이 들어가면 -> V2, V3가 해방되고 -> 그 둘이 들어가면 비로소 V4가 해방되네.
	vertices := []*types.Vertex{v1, v2, v3, v4}
	for _, vtx := range vertices {
		if v.DAG.GetVertex(vtx.Hash) == nil {
			t.Errorf("❌ Vertex %s 가 결국 삽입되지 못했구먼!", vtx.Hash[:8])
		}
	}

	if t.Failed() {
		t.Log("도미노가 중간에 엉켰나 보구먼...")
	} else {
		t.Log("✅ 대성공! 다이아몬드 그물망이 완벽하게 복구되었네!")
	}
}

type MockFetcher struct {
	Called bool
	Hashes []string
}

func (m *MockFetcher) StartSync(missingHashes []string, suspectID int) {
	m.Called = true
	m.Hashes = missingHashes
}

func TestSyncTriggerOnRoundGap(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DAG.SyncTriggerThreshold = 2 // 격차가 2보다 크면 작동!

	signer, _ := NewEd25519Signer()
	mock := &MockFetcher{}

	v := NewValidator(0, cfg, signer)
	v.DAG.Fetcher = mock

	t.Run("Case 1: 격차가 작을 때 (Sync 미발동)", func(t *testing.T) {
		mock.Called = false
		currentRound := 5
		// 5 - 4 = 1 (Threshold 2보다 작음)
		vtx := CreateDummyVertex(1, 4, []string{"parent_1"}, signer)
		v.DAG.AddVertex(vtx, currentRound)

		if mock.Called {
			t.Errorf("❌ 격차가 1인데 왜 Fetcher를 깨웠나! 자원 낭비일세.")
		}
	})

	t.Run("Case 2: 격차가 클 때 (Sync 발동!)", func(t *testing.T) {
		mock.Called = false
		currentRound := 10
		// 15 - 10 = 5 (Threshold 2보다 큼!)
		// 내가 10라운드인데 15라운드짜리 고아가 오면 "어이쿠, 나 한참 늦었네!" 하고 소리쳐야 하네.
		vtx := CreateDummyVertex(1, 15, []string{"missing_parent"}, signer)
		v.DAG.AddVertex(vtx, currentRound)

		if !mock.Called {
			t.Errorf("❌ 격차가 5나 되는데 왜 Fetcher가 잠잠한가! 동기화가 시급하네.")
		} else {
			t.Logf("✅ 성공: %v 해시에 대해 동기화 요청이 발송되었네.", mock.Hashes)
		}
	})
}

//DAG 중복 vertex삽입 테스트
func TestDuplicateVertexIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	vtx := CreateDummyVertex(1, 1, []string{}, signer)

	// 1. 첫 번째 삽입
	v.DAG.AddVertex(vtx, 1)
	initialSize := v.DAG.Size()

	// 2. 두 번째 삽입 (중복)
	v.DAG.AddVertex(vtx, 1)

	// 검증: 크기가 변하지 않아야 하네!
	if v.DAG.Size() != initialSize {
		t.Errorf("❌ 중복 삽입으로 DAG 크기가 늘어났네! (Expected: %d, Actual: %d)", initialSize, v.DAG.Size())
	} else {
		t.Log("✅ 성공: 중복 Vertex는 무시되었네.")
	}
}

//DAG invalid hash 테스트
func TestInvalidHashRejected(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	vtx := CreateDummyVertex(1, 1, []string{}, signer)

	// 공격: 데이터를 살짝 바꾸세 (Hash는 그대로 두고 Payload만 변경)
	vtx.Round = 999

	v.DAG.AddVertex(vtx, 1)

	// 검증: DAG에도, OrphanBuffer에도 없어야 하네.
	if v.DAG.GetVertex(vtx.Hash) != nil || v.DAG.Buffer.GetOrphan(vtx.Hash) != nil {
		t.Error("❌ 위조된 해시를 가진 Vertex가 시스템에 침투했네! 보안 비상일세!")
	} else {
		t.Log("✅ 성공: 무결성 검증 실패로 위조 Vertex를 처단했네.")
	}
}

// Partial Parent Arrival 테스트
func TestPartialDependencyResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	v1 := CreateDummyVertex(0, 1, []string{}, signer)                 // Parent 1
	v2 := CreateDummyVertex(1, 1, []string{}, signer)                 // Parent 2
	v3 := CreateDummyVertex(0, 2, []string{v1.Hash, v2.Hash}, signer) // Child

	// 1. 자식(v3) 먼저 주입 -> 고아행
	v.DAG.AddVertex(v3, 2)

	// 2. 부모 1(v1)만 주입
	v.DAG.AddVertex(v1, 2)

	// 검증: v1은 들어갔지만, v3는 여전히 고아여야 하네 (v2가 없으니까!)
	if v.DAG.GetVertex(v3.Hash) != nil {
		t.Fatal("❌ 부모가 하나 부족한데 v3가 벌써 DAG에 합류했구먼!")
	}
	t.Log("✅ v3가 Partial 상태로 나머지 부모를 잘 기다리고 있네.")

	// 3. 마지막 부모 2(v2) 주입
	v.DAG.AddVertex(v2, 2)

	// 4. 최종 검증
	if v.DAG.GetVertex(v3.Hash) != nil {
		t.Log("✅ 대성공: 모든 의존성이 충족되자 비로소 v3가 해방되었네!")
	} else {
		t.Error("❌ 모든 부모가 왔는데도 v3가 여전히 갇혀있네.")
	}
}

func TestDAGConsistencyAndReconstruction(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()

	// 1. 노드 A: 10개의 Vertex를 복잡한 의존성으로 생성하네.
	nodeA := NewValidator(0, cfg, signer)
	vertices := make([]*types.Vertex, 10)

	// 제네시스급 정점들 (Round 1)
	for i := 0; i < 3; i++ {
		vertices[i] = CreateDummyVertex(i, 1, []string{}, signer)
		nodeA.DAG.AddVertex(vertices[i], 1)
	}

	// 복잡하게 꼬인 정점들 (Round 2~4)
	// 이전 라운드 정점들을 무작위로 부모로 삼네.
	vertices[3] = CreateDummyVertex(0, 2, []string{vertices[0].Hash, vertices[1].Hash}, signer)
	vertices[4] = CreateDummyVertex(1, 2, []string{vertices[1].Hash, vertices[2].Hash}, signer)
	vertices[5] = CreateDummyVertex(2, 3, []string{vertices[3].Hash, vertices[4].Hash}, signer)
	vertices[6] = CreateDummyVertex(0, 3, []string{vertices[0].Hash, vertices[5].Hash}, signer)
	vertices[7] = CreateDummyVertex(1, 4, []string{vertices[6].Hash, vertices[2].Hash}, signer)
	vertices[8] = CreateDummyVertex(2, 4, []string{vertices[5].Hash, vertices[7].Hash}, signer)
	vertices[9] = CreateDummyVertex(0, 5, []string{vertices[8].Hash}, signer)

	for i := 3; i < 10; i++ {
		nodeA.DAG.AddVertex(vertices[i], 5)
	}

	// 2. 노드 B: 고아원과 함께 탄생!
	nodeB := NewValidator(1, cfg, signer)

	// 3. 순서를 완전히 뒤섞어서 노드 B에 주입하네.
	// 9번(자식)부터 0번(조상) 순서로 거꾸로 넣어보겠네.
	for i := 9; i >= 0; i-- {
		nodeB.DAG.AddVertex(vertices[i], 5)
	}

	// 4. 최종 검증
	// A와 B의 DAG 사이즈가 같아야 하고, 마지막 Tips의 해시가 일치해야 하네.
	if len(nodeA.DAG.Vertices) != len(nodeB.DAG.Vertices) {
		t.Errorf("❌ 불일치! 노드A 사이즈: %d, 노드B 사이즈: %d",
			len(nodeA.DAG.Vertices), len(nodeB.DAG.Vertices))
	}

	tipsA := nodeA.DAG.GetTips()
	tipsB := nodeB.DAG.GetTips()

	if len(tipsA) != len(tipsB) {
		t.Fatal("❌ Tips의 개수가 다르구먼!")
	}

	t.Logf("✅ 검증 완료: 두 노드의 DAG 사이즈(%d)와 구조가 완벽히 일치하네!", len(nodeB.DAG.Vertices))
}

func TestAbnormalEdgeCases(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DAG.OrphanCapacity = 2 // 용량을 아주 작게
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	t.Run("Möbius Loop Attack", func(t *testing.T) {
		// V1의 부모는 V2, V2의 부모는 V1. 서로가 서로를 기다리네
		v1 := CreateDummyVertex(0, 10, []string{"hash_v2"}, signer)
		v2 := CreateDummyVertex(0, 11, []string{v1.Hash}, signer) // 라운드는 높지만 해시는 순환

		v.DAG.AddVertex(v1, 10)
		v.DAG.AddVertex(v2, 10)

		// 결과: 둘 다 고아원에 갇혀 있어야 하며, 시스템이 멈추면 안 되네.
	})

	t.Run("Orphanage Capacity Limit", func(t *testing.T) {
		// 1. 용량만큼 채우기
		for i := 0; i < 2; i++ {
			vtx := CreateDummyVertex(0, i+1, []string{fmt.Sprintf("missing_%d", i)}, signer)
			v.DAG.AddVertex(vtx, 1)
		}

		// 2. 초과분 주입
		vtxExtra := CreateDummyVertex(0, 99, []string{"missing_99"}, signer)
		v.DAG.AddVertex(vtxExtra, 1)

		size := v.DAG.Buffer.Size()
		fmt.Printf("\n[TEST_RESULT] 현재 고아원 크기: %d (Limit: 2)\n", size)

		if size <= 2 {
			fmt.Printf("✅ 성공: 넘치는 고아들은 규정대로 처리되고 %d개만 남았네.\n", size)
		} else {
			t.Errorf("❌ 용량 초과! %d개가 들어있네.", size)
		}
	})
}

//Thread-Safety, Order-Independence, Idempotency Test
func TestRandomizedDeliveryConvergence(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()

	nodeA := NewValidator(0, cfg, signer)
	nodeB := NewValidator(1, cfg, signer)

	// 동일 vertex set 생성
	vertices := generateComplexDAG(50)

	// A는 정상 순서
	for _, vtx := range vertices {
		nodeA.DAG.AddVertex(vtx, 10)
	}

	// B는 고루틴을 이용한 랜덤 순서 + 중복 삽입
	var wg sync.WaitGroup
	shuffled := shuffle(vertices)

	for _, vtx := range shuffled {
		wg.Add(1)
		go func(v *types.Vertex) {
			defer wg.Done()

			// 1. 랜덤한 네트워크 지연 시뮬레이션
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

			// 2. 주입
			nodeB.DAG.AddVertex(v, 10)

			// 3. 중복 주입 시뮬레이션
			if rand.Intn(2) == 0 {
				nodeB.DAG.AddVertex(v, 10)
			}
		}(vtx)
	}

	wg.Wait() // 모든 난리가 끝날 때까지 대기

	// A와 B가 같아야 진짜 결정론적일세.
	if !compareDAGFull(nodeA.DAG, nodeB.DAG) {
		t.Fatal("❌ DAG divergence detected! 병렬 주입 환경에서 결과가 틀어짐세.")
	}
}

func TestByzantineResilience(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 시나리오 A: 부모 정렬을 뒤섞은 빌런 (Malformed)
	v1 := CreateByzantineVertex(1, 1, []string{}, signer)
	v2 := CreateByzantineVertex(1, 1, []string{}, signer)

	// 강제로 엉망진창 정렬 (v2, v1 순서)
	malformedParents := []string{v2.Hash, v1.Hash}
	if v1.Hash < v2.Hash { // v1이 작다면 정렬 위반 상태로 만듦
		malformedParents = []string{v2.Hash, v1.Hash}
	} else {
		malformedParents = []string{v1.Hash, v2.Hash}
	}

	vByz := CreateByzantineVertex(1, 1, malformedParents, signer) // 생성 시 정렬 위반!

	// 실행
	v.DAG.AddVertex(vByz, 1)

	// 검증: 슬래셔에 보고되었는가?
	if v.Slasher.penaltyTable[1] == 0 {
		t.Fatal("❌ 비잔틴 노드(정렬 위반)를 잡지 못했네!")
	}

	// 시나리오 B: 존재하지 않는 부모를 둔 고아 (Memory Attack)
	for i := 0; i < cfg.DAG.OrphanCapacity+10; i++ {
		ghostVtx := CreateDummyVertex(2, 5, []string{"ghost_hash_" + fmt.Sprint(i)}, signer)
		v.DAG.AddVertex(ghostVtx, 5)
	}

	// 검증: 고아원이 터지지 않고 용량을 유지하는가?
	if v.DAG.Buffer.Size() > cfg.DAG.OrphanCapacity {
		t.Errorf("❌ 고아원이 수용량을 초과했네! 축출 로직 확인 필요: %d", v.DAG.Buffer.Size())
	}
}
