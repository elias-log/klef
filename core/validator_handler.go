package core

import (
	"arachnet-bft/types"
	"fmt"
)

// 공통 검증 진입점: handle 함수들이 호출하기 전에 거쳐가는 관문이라네.
func (v *Validator) preValidate(msg *types.Message) error {
	// 이제 MessageValidator는 validation 패키지에 있네!
	if val, ok := v.messageValidators[msg.Type]; ok {
		// 여기서 v(Validator)는 ValidatorContext 인터페이스로 전달되네!
		return val.Validate(msg, v)
	}
	// 등록되지 않은 메시지 타입에 대한 처리
	return fmt.Errorf("unknown message type: %s", msg.Type)
}

func (v *Validator) handleFetchRequest(msg *types.Message) {
	// 1. 관문 통과!
	if err := v.preValidate(msg); err != nil {
		fmt.Printf("검증 실패(Req): %v\n", err)
		return
	}

	// 2. 페이로드에서 요청된 해시 목록을 꺼내네.
	req := msg.FetchReq
	var foundVertices []*types.Vertex

	// 3. 내 DAG를 뒤져서 있는 것만 골라 담게나.
	for _, h := range req.MissingHashes {
		if vtx := v.DAG.GetVertex(h); vtx != nil {
			foundVertices = append(foundVertices, vtx)
		}
	}

	// 4. 찾은 게 있다면 답장을 보내야지!
	if len(foundVertices) > 0 {
		v.SendTo(msg.FromID, types.MsgFetchRes, &types.FetchResponse{
			Vertices: foundVertices,
		})
	}
}

func (v *Validator) handleFetchResponse(msg *types.Message) {

	if msg.FetchRes == nil {
		return
	}

	// Log
	fmt.Printf("[DEBUG] Validator %d: handleFetchResponse 시작됨! (Vertex 수: %d)\n", v.ID, len(msg.FetchRes.Vertices))

	// 1. 관문 통과
	if err := v.preValidate(msg); err != nil {
		fmt.Printf("[WARN] Validator %d: FetchResponse validation failed: %v\n", v.ID, err)
		return
	}

	// 2. 페이로드에서 Vertex 뭉치를 꺼내네.
	res := msg.FetchRes

	// 3. 받은 Vertex들을 처리하네.
	for _, vtx := range res.Vertices {
		// Log
		fmt.Printf("[DEBUG] 핸들러 유입 Vertex - Payload 명시 해시: %s, 실제 객체 내부 해시: %s\n", vtx.Hash, vtx.CalculateHash())

		// 1. [체크] 이미 DAG에 있다면, Fetcher에게 또 보낼 필요도 없네.
		if v.DAG.GetVertex(vtx.Hash) != nil {
			continue
		}

		// 2. [저장] 먼저 DAG에 안전하게 모셔두게나.
		// AddVertex가 성공했을 때만 후속 조치를 취하는 게 안전하네.
		v.DAG.AddVertex(vtx, v.Round)

		// 3. [피드백] 이제 데이터가 DAG에 확실히 있으니 Fetcher에게 알려주세.
		// Fetcher가 이 Vertex를 기다리고 있을 수 있으니 채널로 쏴주네.
		// 비동기 처리를 위해 select를 사용하여 채널이 가득 찼을 때의 블로킹을 방지하세.
		select {
		case v.Fetcher.InboundResponse <- vtx:
			// Fetcher가 깨어나서 GetMissingHashes를 하면 방금 넣은 vtx를 발견할 걸세.
		default:
			// Fetcher 채널이 꽉 찼다는 건, 이미 충분히 많은 응답을 처리 중이라는 뜻이니 무시해도 좋네.
		}
	}
}

// routeMessage: 들어온 메시지를 검증하고 적절한 처리기로 배정하네.
func (v *Validator) routeMessage(msg *types.Message) {
	// 0. 피어 라운드 정보 업데이트 (가장 기본 정보)
	v.UpdatePeerRound(msg.FromID, msg.CurrentRound)

	// 1. 공통 관문 (preValidate) 통과 확인
	if err := v.preValidate(msg); err != nil {
		fmt.Printf("[WARN] Validator %d: Pre-validation failed for %s from %d: %v\n",
			v.ID, msg.Type, msg.FromID, err)
		return
	}

	// 2. 타입별 핸들러 호출
	switch msg.Type {
	case types.MsgFetchReq:
		v.handleFetchRequest(msg)
	case types.MsgFetchRes:
		v.handleFetchResponse(msg)
	case types.MsgVertex:
		// Vertex가 직접 들어올 때 처리
		if msg.Vertex != nil {
			v.DAG.AddVertex(msg.Vertex, v.Round)
		}
	case types.MsgVote:
		// TODO: v.handleVote(msg)
	default:
		fmt.Printf("[DEBUG] Validator %d: Received unhandled message type: %s\n", v.ID, msg.Type)
	}
}
