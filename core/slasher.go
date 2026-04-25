// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Slasher monitors and penalizes protocol violations to maintain network integrity.

Key properties:
- Equivocation Detection: Identifies and punishes nodes issuing conflicting votes/vertices.
- Evidence Management: Maintains a verifiable ledger of proofs for external reporting.
- Reputation Tracking: Enforces a penalty-based threshold (Slashing) to ignore malicious nodes.
- Internal/External Integration: Processes both locally detected and peer-reported violations.

Note:
- The Slasher enforces accountability for protocol violations,
  complementing the system’s overall BFT safety guarantees.
- Nodes exceeding the penalty threshold are excluded from consensus participation.
*/

package core

import (
	"fmt"
	"klef/config"
	"klef/pkg/types"
	"sync"
	"time"
)

/// Slasher acts as the judiciary branch of the validator node.
// Anti-replay: keyed by primary proof hash.
// NOTE: Future improvement should canonicalize Evidence identity
// to avoid duplicate processing across permutations.
type Slasher struct {
	mu           sync.RWMutex
	evidences    map[int][]types.Evidence // Validator ID -> List of cryptographic proofs
	penaltyTable map[int]int              // Validator ID -> Accumulated demerit points
	processedEv  map[types.Hash]bool      // Anti-replay: Map of already adjudicated Evidence hashes
	Config       *config.Config           // Security policies and penalty thresholds
}

/// NewSlasher initializes the security engine with protocol configurations.
func NewSlasher(cfg *config.Config) *Slasher {
	return &Slasher{
		evidences:    make(map[int][]types.Evidence),
		penaltyTable: make(map[int]int),
		processedEv:  make(map[types.Hash]bool),
		Config:       cfg,
	}
}

/// HandleEquivocation is triggered when a local node detects a double-vote/double-propose.
/// It generates an immediate cryptographic proof of the violation.
func (s *Slasher) HandleEquivocation(vtx1, vtx2 *types.Vertex) *types.Evidence {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Anti-replay check to ensure we don't process the same violation twice.
	if s.processedEv[vtx1.Hash] {
		// Return existing evidence if possible, or nil.
		return nil
	}

	fmt.Printf("[SLASHER] Critical: Equivocation detected by Validator %d (Round: %d)\n", vtx1.Author, vtx1.Round)

	// Using internal helper to ensure processedEv and penaltyTable stay in sync.
	s.executeSlasher(
		vtx1.Author,
		s.Config.Security.EquivocationPenalty,
		vtx1.Hash,
		types.MsgVertex,
		vtx1,
		vtx2,
		"Local detection: Equivocation (Double Propose/Vote)",
	)

	evList := s.evidences[vtx1.Author]
	return &evList[len(evList)-1]
}

/// ProcessExternalEvidence adjudicates violations reported by other nodes in the network.
func (s *Slasher) ProcessExternalEvidence(ev *types.Evidence) {
	// 1. Logical & cryptographic validation:
	// Ensures the evidence represents a valid equivocation proof
	// (e.g., same author, same round, conflicting payloads, valid signatures).
	if !ev.IsValid() {
		fmt.Printf("[SLASHER] Rejection: Invalid evidence reported against Target %d\n", ev.TargetID)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 2. State Guard: Avoid redundant processing or penalizing already excommunicated nodes.
	if s.penaltyTable[ev.TargetID] >= s.Config.Security.SlashThreshold || s.processedEv[ev.Proof1.Hash] {
		return
	}

	s.executeSlasher(
		ev.TargetID,
		s.Config.Security.EquivocationPenalty,
		ev.Proof1.Hash,
		ev.Type,
		ev.Proof1,
		ev.Proof2,
		ev.Description,
	)
}

/// executeSlasher [Internal] commits the punishment to the local ledger.
func (s *Slasher) executeSlasher(
	target int,
	amount int,
	evHash types.Hash,
	evType types.MessageType,
	p1, p2 *types.Vertex,
	reason string,
) {
	s.penaltyTable[target] += amount
	s.processedEv[evHash] = true

	evidence := types.Evidence{
		TargetID:    target,
		Type:        evType,
		Proof1:      p1,
		Proof2:      p2,
		ReporterID:  s.Config.NodeID,
		Description: reason,
		Timestamp:   time.Now().Unix(),
	}
	s.evidences[target] = append(s.evidences[target], evidence)

	fmt.Printf("[SLASHER] Execution: Validator %d penalized by %d (Total: %d/%d, Reason: %s)\n",
		target, amount, s.penaltyTable[target], s.Config.Security.SlashThreshold, reason)
}

/// GetPenalty retrieves the current accumulated demerit points for a specific validator.
func (s *Slasher) GetPenalty(validatorID int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.penaltyTable[validatorID]
}

/// IsSlashed determines if a node has exceeded the security threshold.
/// Slashed nodes should have their messages ignored by the consensus engine.
func (s *Slasher) IsSlashed(validatorID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.penaltyTable[validatorID] >= s.Config.Security.SlashThreshold
}

/// AddDemerit allows internal modules (e.g., DAG, Orphanage) to report protocol non-compliance.
func (s *Slasher) AddDemerit(author int, amount int, vtx *types.Vertex, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip if the node is already slashed or the evidence has been processed.
	if s.penaltyTable[author] >= s.Config.Security.SlashThreshold || s.processedEv[vtx.Hash] {
		return
	}

	s.executeSlasher(author, amount, vtx.Hash, types.MsgInvalidPayload, vtx, nil, reason)
}
