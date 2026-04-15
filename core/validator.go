//메시지 라우팅 (Voter, Dag, Fetcher로 전달)

package core

import "arachnet-bft/types"

type Validator struct {
	ID         int         // f.Validator.ID 해결
	Round      int         // v.Round 해결
	DAG        *DAG        // f.Validator.DAG 해결
	PeerRounds map[int]int // 노드 ID -> 해당 노드가 알려준 최신 라운드
	// 네트워크 인터페이스를 여기에 연결해야 하네 (가칭)
	// 실제 구현 시에는 network.Sender 인터페이스 등을 추가할 예정이네
}

func NewValidator(id int) *Validator {
	return &Validator{
		ID:         id,
		Round:      0,
		DAG:        NewDAG(), // 여기서 실제 장부(메모리)를 할당해서 꽂아주는 걸세!
		PeerRounds: make(map[int]int),
	}
}

func (v *Validator) UpdatePeerRound(peerID int, round int) {
	if round > v.PeerRounds[peerID] {
		v.PeerRounds[peerID] = round
	}
}

// f.Validator.SendTo(suspectID, req) 해결
func (v *Validator) SendTo(peerID int, msg *types.Message) {
	// 실제 전송 로직 (나중에 네트워크 레이어와 연결할 부분일세)
}

// f.Validator.Broadcast(req) 해결
func (v *Validator) Broadcast(msg *types.Message) {
	// 모든 피어에게 SendTo를 호출하는 반복문이 들어갈 걸세
}

func (v *Validator) handleFetchRequest(msg *types.Message) {
	// 1. 페이로드에서 요청된 해시 목록을 꺼내네.
	req := msg.Payload.(types.FetchRequest)
	var foundVertices []*types.Vertex

	// 2. 내 DAG를 뒤져서 있는 것만 골라 담게나.
	for _, h := range req.MissingHashes {
		if vtx := v.DAG.GetVertex(h); vtx != nil {
			foundVertices = append(foundVertices, vtx)
		}
	}

	// 3. 찾은 게 있다면 답장을 보내야지!
	if len(foundVertices) > 0 {
		response := &types.Message{
			FromID:       v.ID,
			CurrentRound: v.Round,
			Type:         types.MsgFetchRes,
			Payload:      types.FetchResponse{Vertices: foundVertices},
		}
		v.SendTo(msg.FromID, response)
	}
}

func (v *Validator) handleFetchResponse(msg *types.Message) {
	// 1. 페이로드에서 Vertex 뭉치를 꺼내네.
	res := msg.Payload.(types.FetchResponse)

	// 2. 받은 Vertex들을 하나씩 DAG에 삽입 시도하네.
	for _, vtx := range res.Vertices {
		// 이미 DAG에 있는지 확인 (중복 삽입 방지)
		if v.DAG.GetVertex(vtx.Hash) != nil {
			continue
		}

		// 3. 정식 삽입 로직 호출!
		// 이 안에서 Buffer를 확인하고 자식들을 깨우는 도미노가 시작될 걸세.
		v.DAG.AddVertex(vtx, v.Round)
	}
}
