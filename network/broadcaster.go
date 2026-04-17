package network

import (
	"arachnet-bft/types"
	"fmt"
)

type Broadcaster interface {
	// Broadcast: 전체 방송 (Gossip 등)
	Broadcast(msgType types.MessageType, payload interface{})

	// Multicast: 특정 커미티에게만 쏘는 Multicast
	Multicast(committee []int, msgType types.MessageType, payload interface{})

	// SendTo: 특정 노드에게만 귓속말 (FetchRes 등)
	SendTo(targetID int, msgType types.MessageType, payload interface{})
}

// NetworkManager: 실제 Broadcaster 인터페이스를 구현할 목업 객체일세.
// 지금은 출력만 하지만, 나중에 P2P 로직을 채울 걸세.
type NetworkManager struct {
	SelfID int
}

func NewNetworkManager(id int) *NetworkManager {
	return &NetworkManager{SelfID: id}
}

func (n *NetworkManager) Broadcast(msgType types.MessageType, payload interface{}) {
	// TODO: Gossip 프로토콜 연동
	fmt.Printf("[Network] Node %d: Broadcasting %v\n", n.SelfID, msgType)
}

func (n *NetworkManager) Multicast(committee []int, msgType types.MessageType, payload interface{}) {
	// TODO: 커미티 필터링 전송
	fmt.Printf("[Network] Node %d: Multicasting %v to %v\n", n.SelfID, msgType, committee)
}

func (n *NetworkManager) SendTo(targetID int, msgType types.MessageType, payload interface{}) {
	// TODO: 특정 피어 Unicast
	fmt.Printf("[Network] Node %d: Sending %v to %d\n", n.SelfID, msgType, targetID)
}
