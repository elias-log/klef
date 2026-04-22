// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator state accessors provide controlled access to internal node state.

Key properties:
- Thread-Safety: Protects critical mutable states using mutexes where required.
- Abstraction: Shields internal sub-modules from direct dependency on the Validator struct.
- Invariant Protection: Prevents double-proposing through monotonic round tracking.

Note:
- Several methods act as stubs for Phase 4/5 integration (Rate Limiting, Anti-Replay).
- Serves as the primary interface for the Consensus engine to query node capabilities.
*/

package core

import (
	"arachnet-bft/consensus"
	"arachnet-bft/types"
)

/// GetCurrentRound returns the node's current logical clock height in the DAG.
func (v *Validator) GetCurrentRound() int {
	return v.Round
}

/// IsKnownVertex checks the local DAG storage for the existence of a specific hash.
func (v *Validator) IsKnownVertex(hash string) bool {
	return v.DAG.GetVertex(hash) != nil
}

/// Sign generates a cryptographic signature for the provided data using the node's private key.
func (v *Validator) Sign(data string) types.Signature {
	return v.Signer.Sign([]byte(data))
}

/// GetQuorumPolicy retrieves the current consensus rules for threshold calculation.
func (v *Validator) GetQuorumPolicy() consensus.QuorumPolicy {
	return v.Policy
}

/// QuorumThreshold returns the minimum number of votes required for each consensus hierarchy.
// TODO(Consensus): Support context-aware thresholding for multiple QC types (Global/Committee).
func (v *Validator) QuorumThreshold() int {
	return v.Policy.GetQuorumSize(types.QCGlobal)
}

/// GetID returns the unique integer identifier of this validator node.
func (v *Validator) GetID() int {
	return v.ID
}

/// AlreadyProposed verifies if the node has already issued a proposal for the given round or higher.
/// This prevents equivocation and ensures consistency with deterministic finalization rules.
func (v *Validator) AlreadyProposed(round int) bool {
	v.proposalMu.Lock()
	defer v.proposalMu.Unlock()
	return round <= v.lastProposedRound
}

/// MarkAsProposed records the successful broadcast of a proposal for a specific round.
/// It ensures the proposal history only moves forward monotonically.
func (v *Validator) MarkAsProposed(round int) {
	v.proposalMu.Lock()
	defer v.proposalMu.Unlock()
	if round > v.lastProposedRound {
		v.lastProposedRound = round
	}
}

/// IsDuplicateMessage provides anti-replay protection by checking message history.
// TODO(Safety): Implement persistent MessageHistory ledger in Phase 5.
func (v *Validator) IsDuplicateMessage(peerID int, seq uint64) bool {
	return false
}

/// UpdateAndCheckRateLimit enforces resource quotas per peer to mitigate DoS attacks.
// TODO(Stability): Implement sliding-window rate limiting based on Config quotas.
func (v *Validator) UpdateAndCheckRateLimit(peerID int) bool {
	return true
}

/// IsFinalized determines whether a vertex has been anchored and included
/// in the globally deterministic outcome.
/// Note: Finality is derived from DAG anchoring, not local observation.
// TODO(Finality): Integrate with the Finalizer's anchor-tracking logic in Phase 8.
func (v *Validator) IsFinalized(hash string) bool {
	return false
}
