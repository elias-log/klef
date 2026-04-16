package network

import "arachnet-bft/types"

type Broadcaster interface {
	// 전체 방송 (Gossip 등)
	Broadcast(msgType types.MessageType, payload interface{})

	// 특정 커미티에게만 쏘는 Multicast
	Multicast(committee []int, msgType types.MessageType, payload interface{})

	// 특정 노드에게만 귓속말 (FetchRes 등)
	SendTo(targetID int, msgType types.MessageType, payload interface{})
}
