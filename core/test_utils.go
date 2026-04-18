package core

import (
	"arachnet-bft/types"
	"time"
)

// CreateDummyVertex: 라운드와 부모를 지정하면 테스트용 Vertex를 만들어주네.
func CreateDummyVertex(author int, round int, parents []string, signer *Ed25519Signer) *types.Vertex {
	vtx := &types.Vertex{
		Author:    author,
		Round:     round,
		Parents:   parents,
		ParentQC:  &types.QC{},
		Timestamp: time.Now().Unix(),
		Payload:   [][]byte{[]byte("test_transaction")},
	}

	// 1. 먼저 해시를 계산하네.
	vtx.Hash = vtx.CalculateHash()

	// 2. 계산된 해시에 서명을 입히네.
	if signer != nil {
		sig := signer.Sign([]byte(vtx.Hash))
		vtx.Signature = []byte(sig)
	}

	return vtx
}

// InjectFetchResponse: 특정 검증인에게 가짜 FetchResponse를 주입하네.
func InjectFetchResponse(v *Validator, vertices []*types.Vertex) {
	msg := &types.Message{
		FromID:   999,
		Type:     types.MsgFetchRes,
		FetchRes: &types.FetchResponse{Vertices: vertices},
	}
	v.InboundMsg <- msg
}
