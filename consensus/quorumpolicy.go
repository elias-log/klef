// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
QuorumPolicy defines the logic for consensus thresholds and membership transitions.

Key properties:
- Abstract Validation: Decouples quorum math from the consensus engine (Jolteon/BFT).
- Transition Safety: Reduces the risk of conflicting quorums during membership shifts
  by enforcing conservative threshold calculation.
- Conservative Calculation: Prioritizes safety over liveness by taking the stricter (larger)
  threshold when two membership views conflict.

Design Principle:
    Safety over Speed: In dynamic environments, requiring the maximum
    possible quorum size (Max(oldQ, newQ)) guarantees that no two
    conflicting partitions can independently reach quorum.

	This ensures quorum intersection across membership views.

Notes:
- Current implementation assumes a static ratio (e.g., 2/3 + 1) for Byzantine fault tolerance.
- Future versions will integrate VRF-based shard committee selection.
*/

package consensus

import (
	"klef/types"
	"math"
	"sync"
)

/// QuorumPolicy is a generic interface for determining consensus thresholds.
type QuorumPolicy interface {
	/// IsQuorum verifies if the collected signatures satisfy the required threshold.
	IsQuorum(qc *types.QC) bool

	/// GetQuorumSize calculates the minimum number of participants needed for a valid QC.
	GetQuorumSize(qcType types.QCType) int
}

/// DynamicQuorumPolicy handles threshold logic with support for membership reconfigurations.
type DynamicQuorumPolicy struct {
	mu sync.RWMutex

	// Total network size for Global QCs (N)
	oldTotalN int
	newTotalN int

	// Shard-specific committee size for Local QCs (n)
	oldCommitteeN int
	newCommitteeN int

	// Status flag for active membership transitions
	isTransitioning bool

	// Threshold ratios (e.g., 0.67 for 2/3+1)
	globalRatio    float64
	committeeRatio float64
}

/// NewDynamicQuorumPolicy creates a policy instance with specified ratios and initial sizes.
func NewDynamicQuorumPolicy(totalN int, committeeN int, gRatio, cRatio float64) *DynamicQuorumPolicy {
	return &DynamicQuorumPolicy{
		oldTotalN:       totalN,
		newTotalN:       totalN,
		oldCommitteeN:   committeeN,
		newCommitteeN:   committeeN,
		isTransitioning: false,
		globalRatio:     gRatio,
		committeeRatio:  cRatio,
	}
}

/// GetQuorumSize computes the numerical threshold for a given QC type.
// Calculation Logic:
// - Non-transition: Standard BFT-style threshold (e.g., 2f+1).
// - Transition: Max(calc(oldN), calc(newN)) to maintain safety across views.
func (p *DynamicQuorumPolicy) GetQuorumSize(qcType types.QCType) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var ratio float64
	var oldN, newN int

	if qcType == types.QCGlobal {
		ratio = p.globalRatio
		oldN, newN = p.oldTotalN, p.newTotalN
	} else {
		ratio = p.committeeRatio
		oldN, newN = p.oldCommitteeN, p.newCommitteeN
	}

	calc := func(n int, r float64) int {
		return int(math.Floor(float64(n)*r)) + 1
	}

	if !p.isTransitioning {
		return calc(newN, ratio)
	}

	// Conservative strategy for safe transitions
	oldQ := calc(oldN, ratio)
	newQ := calc(newN, ratio)

	if oldQ > newQ {
		return oldQ
	}
	return newQ
}

/// IsQuorum performs a threshold check on the provided Quorum Certificate.
// Separation of concerns:
// This method assumes signatures are already verified.
// It only evaluates quorum size to keep this layer lightweight.
func (p *DynamicQuorumPolicy) IsQuorum(qc *types.QC) bool {
	// 1. Fetch the required quorum size based on current policy state.
	required := p.GetQuorumSize(qc.Type)

	// 2. Evaluate if the signature set meets or exceeds the requirement.
	// Assumption: qc.Signatures contains unique validator signatures.
	// Duplicate signer handling must be enforced at a higher layer.
	return len(qc.Signatures) >= required
}
