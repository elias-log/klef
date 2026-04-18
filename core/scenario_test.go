package core

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"testing"
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
