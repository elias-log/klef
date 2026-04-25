// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Replica state accessors expose controlled runtime state to consensus
and orchestration subcomponents.

Key properties:
- Thread-Safety: Protects mutable consensus state through internal locking.
- Encapsulation: Hides internal runtime state behind stable accessors.
- Invariant Protection: Prevents double-proposal and preserves monotonic progress.

Note:
- Some methods are placeholders for later phases
  (anti-replay, rate limiting, finalization tracking).
- These accessors define the runtime surface used by consensus components.
*/

package replica

import (
	"context"
	"klef/config"
	"klef/consensus"
	"klef/pkg/types"
)

func (r *Replica) RuntimeContext() context.Context {
	return r.ctx
}

func (r *Replica) ReplicaConfig() *config.Config {
	return r.cfg
}

/// CurrentRound returns the node's current logical clock height in the DAG.
func (r *Replica) CurrentRound() int {
	return r.round
}

/// IsKnownVertex checks the local DAG storage for the existence of a specific hash.
func (r *Replica) IsKnownVertex(hash types.Hash) bool {
	return r.dag.GetVertex(hash) != nil
}

/// Sign generates a cryptographic signature for the provided data using the node's private key.
func (r *Replica) Sign(data []byte) types.Signature {
	return r.signer.Sign(data)
}

/// QuorumPolicy retrieves the current consensus rules for threshold calculation.
func (r *Replica) QuorumPolicy() consensus.QuorumPolicy {
	return r.policy
}

/// QuorumThreshold returns the minimum number of votes required for each consensus hierarchy.
// TODO(Consensus): Support context-aware thresholding for multiple QC types (Global/Committee).
func (r *Replica) QuorumThreshold() int {
	return r.policy.QuorumSize(types.QCGlobal)
}

/// ID returns the unique integer identifier of this validator node.
func (r *Replica) ID() int {
	return r.id
}

/// AlreadyProposed verifies if the node has already issued a proposal for the given round or higher.
/// This prevents equivocation and ensures consistency with deterministic finalization rules.
func (r *Replica) AlreadyProposed(round int) bool {
	r.proposalMu.Lock()
	defer r.proposalMu.Unlock()
	return round <= r.lastProposedRound
}

/// MarkAsProposed records the successful broadcast of a proposal for a specific round.
/// It ensures the proposal history only moves forward monotonically.
func (r *Replica) MarkAsProposed(round int) {
	r.proposalMu.Lock()
	defer r.proposalMu.Unlock()
	if round > r.lastProposedRound {
		r.lastProposedRound = round
	}
}

/// IsDuplicateMessage provides anti-replay protection by checking message history.
// TODO(Safety): Implement persistent MessageHistory ledger in Phase 5.
func (r *Replica) IsDuplicateMessage(peerID int, seq uint64) bool {
	return false
}

/// UpdateAndCheckRateLimit enforces resource quotas per peer to mitigate DoS attacks.
// TODO(Stability): Implement sliding-window rate limiting based on Config quotas.
func (r *Replica) UpdateAndCheckRateLimit(peerID int) bool {
	return true
}

/// IsFinalized determines whether a vertex has been anchored and included
/// in the globally deterministic outcome.
/// Note: Finality is derived from DAG anchoring, not local observation.
// TODO(Finality): Integrate with the Finalizer's anchor-tracking logic in Phase 8.
func (r *Replica) IsFinalized(hash types.Hash) bool {
	return false
}
