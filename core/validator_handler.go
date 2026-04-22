// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator Message Handlers define the ingress pipeline and routing logic.

Key properties:
- Strategy-based Validation: Uses MessageValidators to decouple type-specific checks.
- Event-Driven Flow: Bridges network messages with internal components (DAG, Fetcher, PendingMgr).
- Data Consistency: Assists in resolving pending causal dependencies via DAG insertion and Fetcher feedback.

Note:
- This is the primary entry point for all asynchronous P2P traffic.
- Future: Integration with the Slasher will occur in preValidate to penalize malformed traffic.
*/

package core

import (
	"arachnet-bft/types"
	"fmt"
)

/// preValidate executes type-specific validation strategies.
/// It acts as the first line of defense against protocol violations.
/// Unknown message types are treated as protocol violations.
func (v *Validator) preValidate(msg *types.Message) error {
	if val, ok := v.messageValidators[msg.Type]; ok {
		return val.Validate(msg, v)
	}
	return fmt.Errorf("unknown message type: %s", msg.Type)
}

/// handleFetchRequest processes inbound requests for missing DAG vertices.
/// It retrieves available vertices from the local DAG and sends them back to the requester.
/// Note: Partial responses are allowed; only locally available vertices are returned.
func (v *Validator) handleFetchRequest(msg *types.Message) {
	req := msg.FetchReq
	if req == nil {
		return
	}

	var foundVertices []*types.Vertex

	// Scan local DAG for requested hashes.
	for _, h := range req.MissingHashes {
		if vtx := v.DAG.GetVertex(h); vtx != nil {
			foundVertices = append(foundVertices, vtx)
		}
	}

	// Dispatch response if any vertices are found.
	if len(foundVertices) > 0 {
		v.SendTo(msg.FromID, types.MsgFetchRes, &types.FetchResponse{
			Vertices: foundVertices,
		})
	}
}

/// handleFetchResponse processes inbound vertex data requested via Fetcher.
/// 1. Integrates newly received vertices into the DAG.
/// 2. Resolves pending dependencies via PendingManager cleanup.
/// 3. Feeds the Fetcher to unblock dependent fetch operations.
func (v *Validator) handleFetchResponse(msg *types.Message) {

	res := msg.FetchRes
	if res == nil {
		return
	}
	fmt.Printf("[DEBUG] Validator %d: Handling FetchResponse with %d vertices\n", v.ID, len(res.Vertices))

	for _, vtx := range res.Vertices {
		// 1. Redundancy Check: Ignore if already exists in local storage.
		if v.DAG.GetVertex(vtx.Hash) != nil {
			continue
		}

		// 2. Lifecycle Cleanup: Mark the data as 'retrieved' in PendingManager.
		v.pendingMgr.Remove(vtx.Hash)

		// 3. DAG Integration: Attempt to insert vertex (may trigger orphan logic).
		v.DAG.AddVertex(vtx, v.Round)

		// 4. Non-blocking feedback: avoids stalling the handler under high load.
		//    Dropped signals imply the system is already saturated with valid data.
		select {
		case v.Fetcher.InboundResponse <- vtx:
		default:
			// Buffer full indicates the system is already saturated with valid responses.
		}
	}
}

/// routeMessage validates and dispatches incoming messages to their respective handlers.
func (v *Validator) routeMessage(msg *types.Message) {
	// Opportunistically updates peer round information based on observed messages.
	v.UpdatePeerRound(msg.FromID, msg.CurrentRound)

	// Phase 1: Security & Protocol Validation.
	if err := v.preValidate(msg); err != nil {
		fmt.Printf("[WARN] Validator %d: Pre-validation failed for %s from %d: %v\n",
			v.ID, msg.Type, msg.FromID, err)
		// TODO(Safety): Slasher should downgrade the peer's reputation here.
		return
	}

	// Phase 2: Logic-specific Routing.
	switch msg.Type {
	case types.MsgFetchReq:
		v.handleFetchRequest(msg)
	case types.MsgFetchRes:
		v.handleFetchResponse(msg)
	case types.MsgVertex:
		// Direct DAG insertion
		// dependency resolution and orphan handling are delegated to the DAG layer.
		if msg.Vertex != nil {
			v.DAG.AddVertex(msg.Vertex, v.Round)
		}
	case types.MsgVote:
		// TODO(Consensus): v.handleVote(msg)
	default:
		fmt.Printf("[DEBUG] Validator %d: Received unhandled message type: %s\n", v.ID, msg.Type)
	}
}
