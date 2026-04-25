// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
VotePool is a placeholder for vote aggregation logic.

Current status:
- Minimal structure for round-based vote storage.
- Intended to evolve into a quorum-aware aggregation module.

Design intent:
- Collect and manage votes per round.
- Support future quorum formation and validation logic.

Note:
- Implementation is intentionally incomplete at this stage.
- Will be expanded in later consensus phases.
*/

package core

import (
	"klef/pkg/types"
	"sync"
)

type VotePool struct {
	mu    sync.RWMutex
	votes map[int][]*types.Message // Round -> Votes
}

func NewVotePool() *VotePool {
	return &VotePool{
		votes: make(map[int][]*types.Message),
	}
}

func (vp *VotePool) GetVotesByRound(round int) []*types.Message {
	vp.mu.RLock()
	defer vp.mu.RUnlock()
	return vp.votes[round]
}
