// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator Consensus extensions handle threshold detection and voting logic.

Key properties:
- Quorum Detection: Scans the VotePool to identify rounds that have achieved a threshold.
- Policy Integration: Dynamically applies quorum rules (e.g., Global vs. Committee).
- Quorum Readiness: Detects when a round satisfies the threshold required for progression.

Note:
- Current implementation focuses on Global DAG consensus (2f+1).
*/

package core

import (
	"arachnet-bft/types"
)

/// GetReadyQuorum evaluates the VotePool to determine if the required threshold
/// has been met for the current logical round.
/// Serves as a trigger candidate for advancing consensus state (e.g., proposal or commit).
///
/// Returns the set of votes and the corresponding round index if the quorum threshold is met;
/// otherwise, returns nil and 0.
// TODO: Support multi-round evaluation (e.g., pipelined consensus or future rounds)
func (v *Validator) GetReadyQuorum() ([]*types.Message, int) {
	// 1. Target the current observation round.
	// Assumes evaluation is performed on the current local round only.
	round := v.Round
	votes := v.votePool.GetVotesByRound(round)

	// 2. Retrieve the consensus policy defined during initialization.
	policy := v.GetQuorumPolicy()

	// TODO(Consensus): Support dynamic policy selection based on the specific
	// QC Type (Global/Committee) to enable Jolteon-style optimizations.
	requiredQuorum := policy.GetQuorumSize(types.QCGlobal)

	// 3. Threshold Verification: The transition from 'Pending' to 'Committed'.
	// Assumes VotePool enforces uniqueness and validity of votes.
	if len(votes) >= requiredQuorum {
		return votes, round
	}

	return nil, 0
}
