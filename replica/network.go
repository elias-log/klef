// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Replica network accessors provide high-level messaging primitives.

Key properties:
- Message Dissemination: Supports Broadcast, Multicast, and Unicast patterns.
- Abstraction: Decouples consensus logic from the underlying P2P transport layer.
- Future-Proofing: Designed to bridge with the 'PeerManager' and 'NetworkStack'.

Note:
- Current implementations are simulation stubs for message relay.
- Actual wire-protocol serialization and peer routing will be integrated in Phase 3.
- These methods are non-blocking assumptions in current design.
  Future implementations must ensure network calls do not block consensus-critical paths.
*/

package replica

import (
	"fmt"
	"klef/pkg/types"
)

/// Broadcast disseminates a message to all registered peers in the network.
func (r *Replica) Broadcast(msgType types.MessageType, payload interface{}) {
	r.peersMu.RLock()
	peers := make([]int, 0, len(r.peers))
	for id := range r.peers {
		peers = append(peers, id)
	}
	r.peersMu.RUnlock()

	for _, peerID := range peers {
		r.SendTo(peerID, msgType, payload)
	}
}

/// SendTo dispatches a message to a specific target peer.
/// This method acts as the primary egress point for serialized P2P traffic.
// TODO(Network): payload is intentionally abstract and will be serialized at the network layer.
func (r *Replica) SendTo(targetID int, msgType types.MessageType, payload interface{}) {
	// TODO(Network): Bridge this call with the actual P2P layer or PeerManager.
	// Currently simulates the transmission of consensus data.
	// TODO: Replace with structured logger (zap/logrus) in production
	fmt.Printf(
		"[RELAY] Replica %d -> Peer %d: [%v] Transmission initiated.\n",
		r.id,
		targetID,
		msgType,
	)
}

/// Multicast sends a message to a specific subset of peers, typically a committee.
func (r *Replica) Multicast(committee []int, msgType types.MessageType, payload interface{}) {
	// Assumes committee membership is pre-validated and trusted.
	for _, peerID := range committee {
		r.SendTo(peerID, msgType, payload)
	}
}
