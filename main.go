package main

import (
	"arachnet-bft/config"
	"arachnet-bft/core"
	"arachnet-bft/types"
	"context"
	"fmt"
	"time"
)

// MockSigner: skeleton 실행용
type MockSigner struct{}

func (m *MockSigner) Sign(data []byte) types.Signature {
	return types.Signature("fake_sig_data")
}
func (m *MockSigner) GetPublicKey() types.PublicKey {
	return types.PublicKey("mock_public_key")
}
func (m *MockSigner) Verify(data []byte, sig types.Signature, pub types.PublicKey) bool {
	return true
}

func main() {
	// 1. 환경 설정 및 Validator 생성
	cfg := config.DefaultConfig()
	cfg.Sync.Step1Timeout = 1 * time.Second

	signer := &MockSigner{}
	v := core.NewValidator(0, cfg, signer)

	// 2. 엔진 시동
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v.Start(ctx)

	// 피어 등록 (동기화 대상)
	v.AddPeer(99)

	fmt.Println("=== [SYSTEM] Arachnet-BFT Engine Initialized ===")

	// 3. 테스트용 Vertex 생성
	/*mockQC := &types.QC{
		Type:       types.QCGlobal,
		VertexHash: "genesis_hash_000",
		Round:      0,
		ProposerID: 99,
		Signatures: map[int][]byte{
			1: []byte("sig1"),
			2: []byte("sig2"),
			3: []byte("sig3"), // 2f+1를 채웠다고 가정
		},
	}*/

	fakeVtx := &types.Vertex{
		Author:    99,
		Round:     0,
		Parents:   []string{},
		ParentQC:  nil,
		Timestamp: time.Now().Unix(),
		Payload:   [][]byte{[]byte("genesis_tx")},
	}

	fakeVtx.Hash = fakeVtx.CalculateHash()
	targetHash := fakeVtx.Hash

	fmt.Printf("[INFO] 생성된 테스트 Vertex 해시: %s\n", targetHash)

	// 4. 상황 연출: 동기화 트리거
	fmt.Printf("[Action] 해시 [%s]에 대한 동기화 고루틴 가동!\n", targetHash)
	v.Fetcher.StartSync([]string{targetHash}, 99)

	// 5. 네트워크 응답 시뮬레이션
	time.Sleep(2 * time.Second) // Fetcher가 요청을 보낼 시간을 잠시 주세

	resMsg := &types.Message{
		FromID:       99,
		Type:         types.MsgFetchRes,
		CurrentRound: 1,
		Payload:      types.FetchResponse{Vertices: []*types.Vertex{fakeVtx}},
	}

	fmt.Println("[Event] 외부 응답(MsgFetchRes)이 InboundMsg 채널로 유입됨.")
	v.InboundMsg <- resMsg

	// 6. 검증
	time.Sleep(500 * time.Millisecond)

	fmt.Println("\n=== [Final Verification] ===")
	if foundVtx := v.DAG.GetVertex(targetHash); foundVtx != nil {
		fmt.Printf("✅ 성공: Vertex [%s]가 DAG에 안전하게 안착했네!\n", targetHash[:12])
		fmt.Printf("   - 생성자 ID: %d\n", foundVtx.Author)
		fmt.Printf("   - 포함된 트랜잭션 수: %d\n", len(foundVtx.Payload))
		fmt.Printf("   - 부모 QC 라운드: %d\n", foundVtx.ParentQC.Round)
	} else {
		fmt.Println("❌ 실패: 데이터가 실종되었네... 미안하네.")
	}

	fmt.Println("===========================================")
	time.Sleep(1 * time.Second)
}
