//누락된 Vertex를 이웃 노드에게 요청하여 DAG의 구멍을 메움.

package core

import (
	"arachnet-bft/types"
	"time"
)

type Fetcher struct {
	InboundResponse chan *types.Vertex // 요청한 데이터를 받을 채널
	Validator       *Validator         // 부모 노드 정보와 통신 인터페이스를 쓰기 위함일세
}

// RequestMissing: 특정 해시의 Vertex가 없을 때 네트워크에 요청을 보냄
func (f *Fetcher) Request(missingHashes []string, suspectID int) {
	// 1. suspectID (자식을 보낸 놈)에게 먼저 개별적으로 물어보네
	req := &types.Message{
		FromID:  f.Validator.ID,
		Type:    "FETCH_REQ",
		Payload: types.FetchRequest{MissingHashes: missingHashes},
	}

	go func() {
		f.Validator.SendTo(suspectID, req)

		// 2. 잠시 기다려보고 (0.5초)
		time.Sleep(500 * time.Millisecond)

		// 3. 아직도 부모가 안 왔다면? (DAG 상태 확인 후 브로드캐스트)
		if f.Validator.DAG.StillMissing(missingHashes) {
			f.Validator.Broadcast(req)
		}
	}()
}
