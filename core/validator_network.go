package core

import (
	"arachnet-bft/types"
	"fmt"
)

// Broadcast: Broadcaster 인터페이스 규격 (types.MessageType, interface{}) 준수
func (v *Validator) Broadcast(msgType types.MessageType, payload interface{}) {
	v.peersMu.RLock()
	defer v.peersMu.RUnlock()

	for peerID := range v.Peers {
		v.SendTo(peerID, msgType, payload)
	}
}

// SendTo: Broadcaster 인터페이스 규격 준수
func (v *Validator) SendTo(targetID int, msgType types.MessageType, payload interface{}) {
	// 여기서 나중에 NetworkManager나 실제 P2P 레이어를 호출하면 되네.
	// 지금은 Proposer가 던진 데이터를 전송하는 시뮬레이션만 하세.
	fmt.Printf("[RELAY] Validator %d -> Peer %d: [%v] 전송 시도\n", v.ID, targetID, msgType)
}

// Multicast: Broadcaster 인터페이스 규격 준수
func (v *Validator) Multicast(committee []int, msgType types.MessageType, payload interface{}) {
	for _, peerID := range committee {
		v.SendTo(peerID, msgType, payload)
	}
}
