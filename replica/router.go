// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Replica message handlers define the ingress pipeline and routing logic.

Key properties:
- Strategy-based Validation: Uses MessageValidators for type-specific checks.
- Event-Driven Flow: Bridges network messages with internal runtime components.
- Dependency Resolution: Coordinates DAG insertion and fetch lifecycle cleanup.

Note:
- This is the primary ingress point for asynchronous network traffic.
- Routing decisions belong to Replica orchestration, while validation logic
  remains delegated to core validation components.
*/

package replica

import (
	"fmt"
	"klef/core/validation"
	"klef/pkg/types"
)

func (r *Replica) registerValidators() {
	r.messageValidators[types.MsgFetchReq] = &validation.FetchRequestValidator{
		MaxRequestHashes: r.cfg.Request.MaxFetchRequestHashes,
	}
	r.messageValidators[types.MsgFetchRes] = &validation.FetchResponseValidator{
		MaxVertexCount: r.cfg.Request.MaxFetchResponseVtx,
	}
}

/// preValidate executes type-specific validation strategies.
/// It acts as the first line of defense against protocol violations.
/// Unknown message types are treated as protocol violations.
func (r *Replica) preValidate(msg *types.Message) error {
	if val, ok := r.messageValidators[msg.Type]; ok {
		return val.Validate(msg, r)
	}
	return fmt.Errorf("unknown message type: %s", msg.Type)
}

/// handleFetchRequest processes inbound requests for missing DAG vertices.
/// It retrieves available vertices from the local DAG and sends them back to the requester.
/// Note: Partial responses are allowed; only locally available vertices are returned.
func (r *Replica) handleFetchRequest(msg *types.Message) {
	req := msg.FetchReq
	if req == nil {
		return
	}

	var foundVertices []*types.Vertex

	// Scan local DAG for requested hashes.
	for _, h := range req.MissingHashes {
		if vtx := r.dag.GetVertex(h); vtx != nil {
			foundVertices = append(foundVertices, vtx)
		}
	}

	// Dispatch response if any vertices are found.
	if len(foundVertices) > 0 {
		r.SendTo(msg.FromID, types.MsgFetchRes, &types.FetchResponse{
			Vertices: foundVertices,
		})
	}
}

/// handleFetchResponse processes inbound vertex data requested via Fetcher.
/// 1. Integrates newly received vertices into the DAG.
/// 2. Resolves pending dependencies via PendingManager cleanup.
/// 3. Feeds the Fetcher to unblock dependent fetch operations.
func (r *Replica) handleFetchResponse(msg *types.Message) {

	res := msg.FetchRes
	if res == nil {
		return
	}
	fmt.Printf("[DEBUG] Validator %d: Handling FetchResponse with %d vertices\n", r.id, len(res.Vertices))

	for _, vtx := range res.Vertices {
		// 1. Redundancy Check: Ignore if already exists in local storage.
		if r.dag.GetVertex(vtx.Hash) != nil {
			continue
		}

		// 2. Lifecycle Cleanup: Mark the data as 'retrieved' in PendingManager.
		r.pendingMgr.Remove(vtx.Hash)

		// 3. DAG Integration: Attempt to insert vertex (may trigger orphan logic).
		r.dag.AddVertex(vtx, r.round)

		// 4. Non-blocking feedback: avoids stalling the handler under high load.
		//    Dropped signals imply the system is already saturated with valid data.
		select {
		case r.fetcher.InboundResponse <- vtx:
		default:
			// Buffer full indicates the system is already saturated with valid responses.
		}
	}
}

func (r *Replica) RouteMessage(msg *types.Message) {
	r.routeMessage(msg)
}

/// routeMessage validates and dispatches incoming messages to their respective handlers.
func (r *Replica) routeMessage(msg *types.Message) {
	// Opportunistically updates peer round information based on observed messages.
	r.UpdatePeerRound(msg.FromID, msg.CurrentRound)

	// Phase 1: Security & Protocol Validation.
	if err := r.preValidate(msg); err != nil {
		fmt.Printf("[WARN] Validator %d: Pre-validation failed for %s from %d: %v\n",
			r.id, msg.Type, msg.FromID, err)
		// TODO(Safety): Slasher should downgrade the peer's reputation here.
		return
	}

	// Phase 2: Logic-specific Routing.
	switch msg.Type {
	case types.MsgFetchReq:
		r.handleFetchRequest(msg)
	case types.MsgFetchRes:
		r.handleFetchResponse(msg)
	case types.MsgVertex:
		// Direct DAG insertion
		// dependency resolution and orphan handling are delegated to the DAG layer.
		if msg.Vertex != nil {
			r.dag.AddVertex(msg.Vertex, r.round)
		}
	case types.MsgVote:
		// TODO(Consensus): v.handleVote(msg)
	default:
		fmt.Printf("[DEBUG] Validator %d: Received unhandled message type: %s\n", r.id, msg.Type)
	}
}
