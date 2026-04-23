// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Proposer orchestrates the quorum-driven vertex lifecycle.

Key properties:
- Quorum-Driven Causality: Parents are selected not by round index alone,
  but by quorum-backed votes, decoupling causal validity from simple
  round-based ordering.
- Evidence-Backed Proposal: Each vertex includes quorum evidence (QC)
  supporting its causal references, ensuring that its parents are
  justified by sufficient validator agreement.
- Double-Proposal Guard: Strictly prevents equivocation by ensuring an author
  proposes at most once per round.
- Absolute Authenticity: Uses Ed25519 signatures and deterministic hashing
  to ensure vertex integrity during broadcast.
- Vote Consistency:
  The proposer assumes that collected quorum votes are sufficiently
  consistent (i.e., not adversarially fragmented across conflicting parents).
  Full conflict resolution is delegated to the DAG and consensus layers.

Design Principle:
    The Proposer acts as an assembly line that filters and transforms
	network votes into consolidated consensus evidence (QC),
    advancing the DAG state only when causal justification is sufficient.

Notes on Liveness:
    Round transitions are reactive. The node does not advance its local round
    simply by proposing; it waits for the proposed vertex to garner
    sufficient votes to form the next round's QC.
	This prevents speculative round advancement without consensus backing.
*/

package consensus

import (
	"klef/network"
	"klef/types"
	"sync"
	"time"
)

/// Proposer manages the generation and dissemination of new vertices.
type Proposer struct {
	mu            sync.Mutex
	ctx           types.ConsensusContext
	QuorumManager QuorumPolicy
	Broadcaster   network.Broadcaster
}

/// NewProposer initializes the proposer with necessary consensus and network interfaces.
func NewProposer(ctx types.ConsensusContext, qp QuorumPolicy, broadcaster network.Broadcaster) *Proposer {
	return &Proposer{
		ctx:           ctx,
		QuorumManager: qp,
		Broadcaster:   broadcaster,
	}
}

/// Propose identifies a 2f+1 quorum of votes and broadcasts a new vertex to the network.
// Lifecycle:
// 1. Quorum Collection: Fetches votes that have reached the threshold for the target round.
// 2. Safety Check: Prevents redundant proposals (equivocation) for the next round.
// 3. Evidence Assembly: Aggregates signatures into a QC and extracts parent hashes.
// 4. Vertex Construction: Packages transactions, causal links, and evidence into a signed unit.
// 5. Broadcast: Disseminates the vertex to peers to trigger the next voting cycle.
func (p *Proposer) Propose() {

	// 1. Collect Votes (Invariant: Validity)
	// Retrieve 2f+1 votes and the round they represent from the context's vote pool.
	quorumVotes, votedRound := p.ctx.GetReadyQuorum()
	if quorumVotes == nil {
		return
	}

	// 2. Double-Proposal Guard (Invariant: Safety)
	// Ensure we haven't already issued a proposal for the target round.
	nextRound := votedRound + 1
	if p.ctx.AlreadyProposed(nextRound) {
		return
	}

	// 3. Assemble Quorum Certificate and Parent Links
	qc := p.assembleQC(quorumVotes, votedRound)
	parentHashes := p.extractHashesFromVotes(quorumVotes)

	// 4. Vertex Creation
	vtx := &types.Vertex{
		Author:    p.ctx.GetID(),
		Round:     nextRound,
		Parents:   parentHashes,
		ParentQCs: []*types.QC{qc},
		Timestamp: time.Now().UnixMilli(),
		Payload:   p.fetchTransactions(),
	}

	// 5. Deterministic Hashing and Signature
	vtx.Hash = vtx.CalculateHash()
	vtx.Signature = p.ctx.Sign(vtx.Hash.Bytes())

	// 6. Dissemination and State Marking
	p.Broadcaster.Broadcast(types.MsgVertex, vtx)
	p.ctx.MarkAsProposed(nextRound)

	// Note: Round advancement is handled upon receiving valid QCs for this proposal,
	// not immediately after broadcast.
}

/// extractHashesFromVotes deduplicates and extracts vertex hashes from a set of votes.
func (p *Proposer) extractHashesFromVotes(votes []*types.Message) []types.Hash {
	hashSet := make(map[types.Hash]struct{})
	for _, msg := range votes {
		if msg.Vote != nil {
			hashSet[msg.Vote.VertexHash] = struct{}{}
		}
	}

	hashes := make([]types.Hash, 0, len(hashSet))
	for h := range hashSet {
		hashes = append(hashes, h)
	}

	types.SortHashes(hashes)

	return hashes
}

/// fetchTransactions retrieves pending transactions from the mempool.
func (p *Proposer) fetchTransactions() [][]byte {
	// TODO: Integrate with Mempool implementation.
	return [][]byte{}
}

/// assembleQC constructs a Quorum Certificate from a collection of validator votes.
/* [Strategic Evolution Path]
1. Multi-Hash Support: Current representativeHash model will evolve to Merkle Root
   sets to prove DAG-wide causal integrity.
2. BLS Aggregation: Replace map[int][]byte with fixed-size aggregated signatures
   to reduce network bandwidth (O(1) QC size).
3. Bitmask Optimization: Use bitmasks for validator participation tracking
   to minimize header overhead.
*/
func (p *Proposer) assembleQC(votes []*types.Message, round int) *types.QC {
	signatures := make(map[int]types.Signature)
	var representativeHash types.Hash

	for _, msg := range votes {
		if msg.Vote != nil {
			signatures[msg.FromID] = msg.Vote.Signature
			// Temporary and non-deterministic:
			// Uses one arbitrary voted hash as a representative.
			// This does NOT fully capture multi-parent consensus
			// and will be replaced by a deterministic aggregation
			// (e.g., sorted set or Merkle root).
			representativeHash = msg.Vote.VertexHash
		}
	}

	return &types.QC{
		Type:       types.QCGlobal,
		VertexHash: representativeHash,
		Round:      round,
		ProposerID: p.ctx.GetID(),
		Signatures: signatures,
	}
}
