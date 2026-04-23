// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package network

import (
	"fmt"
	"klef/types"
)

/// Broadcaster defines the interface for message propagation across the network.
type Broadcaster interface {
	// Broadcast propagates a message to all connected peers, typically via a Gossip protocol.
	Broadcast(msgType types.MessageType, payload interface{})

	// Multicast targets a specific subset of validators, such as a consensus committee.
	Multicast(committee []int, msgType types.MessageType, payload interface{})

	// SendTo performs a direct unicast to a specific node (e.g., replying to a FetchRequest).
	SendTo(targetID int, msgType types.MessageType, payload interface{})
}

/// NetworkManager provides a concrete implementation of the Broadcaster interface.
/// Currently functions as a mock for local testing, with P2P logic to be integrated later.
type NetworkManager struct {
	SelfID int
}

/// NewNetworkManager initializes a new manager with the given node ID.
func NewNetworkManager(id int) *NetworkManager {
	return &NetworkManager{SelfID: id}
}

/// Broadcast simulates network-wide propagation.
func (n *NetworkManager) Broadcast(msgType types.MessageType, payload interface{}) {
	// TODO: Integrate with a Gossip protocol (e.g., Epidemic broadcast).
	fmt.Printf("[Network] Node %d: Broadcasting %v\n", n.SelfID, msgType)
}

/// Multicast simulates sending messages to a specific committee.
func (n *NetworkManager) Multicast(committee []int, msgType types.MessageType, payload interface{}) {
	// TODO: Implement committee-based filtering and propagation.
	fmt.Printf("[Network] Node %d: Multicasting %v to %v\n", n.SelfID, msgType, committee)
}

/// SendTo simulates a point-to-point unicast message.
func (n *NetworkManager) SendTo(targetID int, msgType types.MessageType, payload interface{}) {
	// TODO: Implement direct peer connection handling.
	fmt.Printf("[Network] Node %d: Sending %v to %d\n", n.SelfID, msgType, targetID)
}
