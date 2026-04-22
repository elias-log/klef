// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
FetchRules defines the validation logic for synchronization messages (FETCH_REQ/RES).

Key properties:
[SECURITY STRATEGY: Resource Protection]
- Tiered Validation: Prioritizes low-cost checks (map lookups) over high-cost checks
  (hashing, signature verification) to mitigate CPU-exhaustion DoS attacks.
- Solicitation Enforcement: Rejects 'Unsolicited Responses' by verifying that every
  incoming vertex was explicitly requested (via PendingManager).
- DoS Mitigation: Strictly limits the batch size of both requests and responses.

[INVARIANTS]
- Non-empty payload: Both requests and responses must contain at least one element.
- Structural Sanity: Performs lightweight structural checks before full DAG validation.
*/

package validation

import (
	"arachnet-bft/types"
	"errors"
	"fmt"
)

/// FetchRequestValidator inspects incoming sync requests from peers.
type FetchRequestValidator struct {
	MaxRequestHashes int // Threshold to prevent massive scanning requests
}

/// Validate scrutinizes an incoming FetchRequest to ensure it conforms to protocol constraints.
/// It primarily serves as a resource-protection layer against Malformed requests and DoS attempts.
func (f *FetchRequestValidator) Validate(msg *types.Message, ctx types.StateReader) error {

	if msg.FetchReq == nil {
		return errors.New("message does not contain a FetchRequest")
	}
	req := msg.FetchReq

	// 1. Minimum sanity check.
	if len(req.MissingHashes) == 0 {
		return errors.New("empty missing hashes in request")
	}

	// 2. DoS defense: Prevent peers from requesting an unreasonable number of hashes.
	if f.MaxRequestHashes > 0 && len(req.MissingHashes) > f.MaxRequestHashes {
		return fmt.Errorf("too many hashes requested: %d", len(req.MissingHashes))
	}

	return nil
}

/// FetchResponseValidator scrutinizes vertices returned in response to our fetch requests.
type FetchResponseValidator struct {
	MaxVertexCount int
}

/// Validate scrutinizes a FetchResponse message containing multiple vertices.
func (f *FetchResponseValidator) Validate(msg *types.Message, ctx types.StateReader) error {
	if msg.FetchRes == nil {
		return errors.New("message does not contain a FetchResponse")
	}
	res := msg.FetchRes

	// 1. Response size validation. (Anti-DoS)
	if len(res.Vertices) == 0 {
		return errors.New("empty fetch response")
	}
	if f.MaxVertexCount > 0 && len(res.Vertices) > f.MaxVertexCount {
		return errors.New("too many vertices in response")
	}

	// 2. Individual Vertex Validation (Tiered by Computational Cost).
	for _, vtx := range res.Vertices {
		// [TIER 1: Lowest Cost] Redundancy Check
		// Ignore if the vertex is already known to the local DAG.
		if ctx.IsKnownVertex(vtx.Hash) {
			continue
		}

		// [TIER 2: Low Cost] Spam Defense (Request-Response Matching)
		// Perform a 'Soft Reject' by skipping vertices that weren't explicitly requested.
		// This handles edge cases where requests expired due to network latency
		// without invalidating the entire response.
		if !ctx.IsRequestPending(vtx.Hash) {
			fmt.Printf("[DEBUG] Unsolicited vertex skipped: %s\n", vtx.Hash[:8])
			continue
		}

		// [TIER 3: Medium Cost] Protocol Bounds Check
		// Verify if the vertex round is within an acceptable temporal window.
		if err := f.validateRoundRange(vtx, ctx); err != nil {
			return err
		}

		// [TIER 4: High Cost] Cryptographic Integrity
		// Defer expensive hash calculations until metadata validation passes.
		if vtx.Hash != vtx.CalculateHash() {
			return fmt.Errorf("hash mismatch for vertex: %s", vtx.Hash)
		}

		// [TIER 5: Structural Check] DAG Invariants
		// Enforce strict causal rules and structural integrity.
		if err := f.validateStructure(vtx); err != nil {
			return err
		}
	}

	return nil
}

/// validateRoundRange ensures the vertex falls within valid past/future round boundaries.
func (f *FetchResponseValidator) validateRoundRange(vtx *types.Vertex, ctx types.StateReader) error {
	currRound := ctx.GetCurrentRound()

	// TODO(Scalability): Transition from hardcoded drift limits to dynamic, context-aware policies.
	// As Arachnet evolves with Sharding and Asynchronous Committees, round drift tolerances
	// should be retrieved via ctx.GetConfig() or adjusted per-mode (Global vs. Local).
	// [Reference: Phase 2 Scalability Roadmap]

	// Max drift allowed for future rounds to accommodate network latency.
	maxFuture := 10
	// Retention window for past rounds to prevent stale data injection.
	maxPast := 50

	if vtx.Round > currRound+maxFuture {
		return fmt.Errorf("vertex %s is too far in the future", vtx.Hash)
	}

	// Strict past-round enforcement only after the initial synchronization phase.
	if currRound > maxPast && vtx.Round < currRound-maxPast {
		return fmt.Errorf("vertex %s is too old", vtx.Hash)
	}

	return nil
}

/// validateStructure verifies parent references and basic vertex composition rules.
func (f *FetchResponseValidator) validateStructure(vtx *types.Vertex) error {
	// Protocol violation: Every vertex except the Genesis must reference at least one parent.
	if vtx.Round > 0 && len(vtx.Parents) == 0 {
		return fmt.Errorf("non-genesis vertex %s has no parents", vtx.Hash)
	}

	parentSet := make(map[string]struct{})
	for _, pHash := range vtx.Parents {
		if pHash == vtx.Hash {
			return fmt.Errorf("circular reference in vertex %s", vtx.Hash)
		}
		if _, exists := parentSet[pHash]; exists {
			return fmt.Errorf("duplicate parent %s in vertex %s", pHash, vtx.Hash)
		}
		parentSet[pHash] = struct{}{}
	}
	return nil
}
