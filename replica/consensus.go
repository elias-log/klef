// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Consensus state accessors provide quorum evaluation over local vote state.

Key properties:
- Quorum Detection: Scans the VotePool for threshold satisfaction.
- Policy Integration: Applies quorum rules based on QC type.
- Progress Readiness: Determines whether local consensus can advance.

Note:
- Quorum requirements may differ depending on the consensus artifact
  (e.g. DataQC vs ExecQC).
- Current implementation evaluates only the current local round.
*/

package replica

import (
	"klef/pkg/types"
)

/// CheckQuorum evaluates whether the current round has reached quorum
/// for the given QC type.
///
/// Returns the collected votes and round number if quorum is satisfied;
/// otherwise returns nil and zero.
func (r *Replica) CheckQuorum(qcType types.QCType) ([]*types.Message, int) {
	// Evaluate only the current local observation round.
	// Assumes evaluation is performed on the current local round only.
	round := r.round
	votes := r.votePool.GetVotesByRound(round)

	// Resolve quorum threshold based on QC semantics.
	requiredQuorum := r.QuorumPolicy().QuorumSize(qcType)

	// Transition condition: quorum satisfied.
	// Assumes VotePool enforces uniqueness and validity of votes.
	if len(votes) >= requiredQuorum {
		return votes, round
	}

	return nil, 0
}
