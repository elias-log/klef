// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package replica

import (
	"context"
	"klef/config"
	"klef/consensus"
	"klef/core"
	"klef/core/validation"
	"klef/pkg/service"
	"klef/pkg/types"
	"math/rand"
	"sync"
	"time"
)

type Replica struct {
	// [Identity]
	id        int
	publicKey types.PublicKey
	cfg       *config.Config
	signer    types.Signer
	rng       *rand.Rand

	// [Core components]
	validator *core.Validator
	fetcher   *core.VertexFetcher // Remote vertex acquisition engine
	dag       *core.DAG           // Causal ordering and data availability layer
	slasher   *core.Slasher       // Misbehavior detection and penalty enforcement

	// [Managers]
	pendingMgr *core.PendingManager // Lifecycle management for out-of-order data requests
	votePool   *core.VotePool       // Ephemeral buffer for BFT vote aggregation

	// [Consensus]
	proposer          *consensus.Proposer
	policy            consensus.QuorumPolicy
	round             int
	lastProposedRound int
	proposalMu        sync.Mutex

	// [Peer state]
	peers      map[int]bool // Peers[nodeID] = isActive
	peerRounds map[int]int  // PeerRounds[nodeID] = the latest round height reported by each node ID.
	peersMu    sync.RWMutex

	// [Validation registry]
	messageValidators map[types.MessageType]validation.MessageValidator

	// [Lifecycle]
	ctx      context.Context    // Root context for graceful degradation
	cancel   context.CancelFunc //
	services []service.Service  //

	// [Network ingress]
	InboundMsg chan *types.Message // Primary ingress for serialized p2p traffic
}

func NewReplica(cfg *config.Config, signer types.Signer) *Replica {
	ctx, cancel := context.WithCancel(context.Background())

	r := &Replica{
		id:        cfg.NodeID,
		publicKey: signer.GetPublicKey(),
		cfg:       cfg,
		signer:    signer,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),

		lastProposedRound: -1,

		peers:      make(map[int]bool),
		peerRounds: make(map[int]int),

		messageValidators: make(map[types.MessageType]validation.MessageValidator),

		ctx:    ctx,
		cancel: cancel,

		InboundMsg: make(chan *types.Message, cfg.Resource.ValidatorChannelSize),
	}

	// Initialize Quorum Policy based on committee configuration
	r.policy = consensus.NewDynamicQuorumPolicy(
		cfg.Consensus.TotalNodes,
		cfg.Consensus.CommitteeNodes,
		cfg.Consensus.GlobalQuorumRatio,
		cfg.Consensus.CommitteeQuorumRatio,
	)

	// Core modules
	r.pendingMgr = core.NewPendingManager(cfg)
	r.votePool = core.NewVotePool()
	r.slasher = core.NewSlasher(cfg)

	r.validator = core.NewValidator(cfg.NodeID, cfg, signer)

	r.fetcher = &core.VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.Resource.FetcherChannelSize),
		Context:         r,
	}

	r.dag = core.NewDAG(r.fetcher, r.slasher, cfg)

	r.validator.DAG = r.dag         // wiring
	r.validator.Slasher = r.slasher // wiring
	r.validator.Policy = r.policy   // wiring

	r.proposer = consensus.NewProposer(
		r,
		r.policy,
		r,
	)

	r.registerValidators()

	r.services = []service.Service{
		r.pendingMgr,
	}

	return r
}
