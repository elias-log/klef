package core

import (
	"arachnet-bft/types"
)

// f.Validator.SendTo(suspectID, req) 해결
func (v *Validator) SendTo(peerID int, msg *types.Message) {
	// 실제 전송 로직 (나중에 네트워크 레이어와 연결할 부분일세)
}

// f.Validator.Broadcast(req) 해결
func (v *Validator) Broadcast(msg *types.Message) {
	// 모든 피어에게 SendTo를 호출하는 반복문이 들어갈 걸세
}
