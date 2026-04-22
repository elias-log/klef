// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator coordinates DAG, Consensus, and Network layers.

Key properties:
- Safety: All incoming messages are pre-validated before state transition.
- Liveness: Event-driven decoupling prevents deadlocks between components.
- Stability: Ingress is rate-limited to maintain bounded resource usage.

Design note:
- This component will evolve into a stateless verification engine,
  with orchestration delegated to a higher-level controller.
*/

package core

import (
	"arachnet-bft/config"
	"arachnet-bft/consensus"
	"arachnet-bft/core/validation"
	"arachnet-bft/types"
	"context"
	"fmt"
	"sync"
)

/// Validator acts as the central coordinator for consensus and state validation.
/// Future Roadmap: Transition into a stateless verification engine while
/// delegating orchestration to a dedicated Service Manager.
type Validator struct {
	// [Identity & Static]
	ID        int
	PublicKey types.PublicKey
	Config    *config.Config

	// [Core Logic Dependencies]
	Signer            types.Signer
	DAG               *DAG           // Causal ordering and data availability layer
	Fetcher           *VertexFetcher // Remote vertex acquisition engine
	Slasher           *Slasher       // Misbehavior detection and penalty enforcement
	messageValidators map[types.MessageType]validation.MessageValidator
	Proposer          *consensus.Proposer
	Policy            consensus.QuorumPolicy

	// [Consensus & Peer State]
	Round             int
	lastProposedRound int
	proposalMu        sync.Mutex
	PeerRounds        map[int]int  // PeerRounds[nodeID] = the latest round height reported by each node ID.
	peersMu           sync.RWMutex //
	Peers             map[int]bool // Peers[nodeID] = isActive

	// [Buffers & Managers]
	pendingMgr *PendingManager // Lifecycle management for out-of-order data requests
	votePool   *VotePool       // Ephemeral buffer for BFT vote aggregation

	// [Control Flow]
	ctx        context.Context     // Root context for graceful degradation
	cancel     context.CancelFunc  //
	InboundMsg chan *types.Message // Primary ingress for serialized p2p traffic
}

/// NewValidator initializes a validator instance with a dynamic quorum policy.
/// It constructs the T-Initialization chain: Config -> Validator -> Sub-modules.
func NewValidator(id int, cfg *config.Config, signer types.Signer) *Validator {
	ctx, cancel := context.WithCancel(context.Background())

	v := &Validator{
		ID:                id,
		Config:            cfg,
		Signer:            signer,
		PublicKey:         signer.GetPublicKey(),
		lastProposedRound: -1,
		ctx:               ctx,
		cancel:            cancel,

		Peers:             make(map[int]bool),
		PeerRounds:        make(map[int]int),
		pendingMgr:        NewPendingManager(cfg),
		votePool:          NewVotePool(),
		messageValidators: make(map[types.MessageType]validation.MessageValidator),
		InboundMsg:        make(chan *types.Message, cfg.Resource.ValidatorChannelSize),
	}

	// Initialize Quorum Policy based on committee configuration
	v.Policy = consensus.NewDynamicQuorumPolicy(
		cfg.Consensus.TotalNodes,
		cfg.Consensus.CommitteeNodes,
		cfg.Consensus.GlobalQuorumRatio,
		cfg.Consensus.CommitteeQuorumRatio,
	)

	// Wire sub-components with circular reference mitigation
	v.Proposer = consensus.NewProposer(v, v.Policy, v)
	v.Slasher = NewSlasher(cfg)
	v.Fetcher = &VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.Resource.FetcherChannelSize),
		Validator:       v,
	}
	v.DAG = NewDAG(v.Fetcher, v.Slasher, cfg)

	v.registerValidators()
	return v
}

/// Start initiates the background maintenance loops and the message routing engine.
func (v *Validator) Start(ctx context.Context) {
	v.ctx, v.cancel = context.WithCancel(ctx)
	go v.pendingMgr.StartCleanupLoop(v.ctx)
	go v.runMessageLoop()
}

/// runMessageLoop executes the primary event loop, routing messages to their
/// respective handlers based on pre-validated types.
func (v *Validator) runMessageLoop() {
	fmt.Printf("[SYSTEM] Validator %d: Message routing loop started.\n", v.ID)

	for {
		select {
		case msg := <-v.InboundMsg:
			v.routeMessage(msg)

		case <-v.ctx.Done():
			fmt.Printf("[SYSTEM] Validator %d: Shutting down...\n", v.ID)
			return
		}
	}
}

func (v *Validator) Stop() {
	v.cancel()
}

func (v *Validator) registerValidators() {
	v.messageValidators[types.MsgFetchReq] = &validation.FetchRequestValidator{
		MaxRequestHashes: v.Config.Request.MaxFetchRequestHashes,
	}
	v.messageValidators[types.MsgFetchRes] = &validation.FetchResponseValidator{
		MaxVertexCount: v.Config.Request.MaxFetchResponseVtx,
	}
}
