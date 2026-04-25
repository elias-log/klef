// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator provides core verification and consensus-adjacent logic.

Key properties:
- Stateless Verification: Performs protocol checks without owning runtime lifecycle.
- Deterministic Inspection: Evaluates DAG state and consensus artifacts.
- Reusable Logic Unit: Embedded into Replica as a pure verification component.

Design note:
- Lifecycle orchestration (startup, shutdown, message routing) is owned by Replica.
*/

package core

import (
	"klef/config"
	"klef/consensus"
	"klef/pkg/types"
	"sync"
)

/// Validator encapsulates protocol verification and local consensus logic.
/// It does not own network ingress or runtime lifecycle.
type Validator struct {
	// [Identity & Static]
	id        int
	PublicKey types.PublicKey
	Config    *config.Config

	// [Core Logic Dependencies]
	Signer  types.Signer
	DAG     *DAG
	Slasher *Slasher
	Policy  consensus.QuorumPolicy

	// [Consensus State]
	Round             int
	lastProposedRound int
	proposalMu        sync.Mutex

	// [Ephemeral State]
	votePool *VotePool
}

/// NewValidator initializes a pure validation engine with core dependencies.
/// Runtime lifecycle ownership is delegated to Replica.
func NewValidator(id int, cfg *config.Config, signer types.Signer) *Validator {
	v := &Validator{
		id:                id,
		Config:            cfg,
		Signer:            signer,
		PublicKey:         signer.GetPublicKey(),
		lastProposedRound: -1,
		votePool:          NewVotePool(),
	}

	// Initialize quorum policy
	v.Policy = consensus.NewDynamicQuorumPolicy(
		cfg.Consensus.TotalNodes,
		cfg.Consensus.CommitteeNodes,
		cfg.Consensus.GlobalQuorumRatio,
		cfg.Consensus.CommitteeQuorumRatio,
	)

	return v
}
