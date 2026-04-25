// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package types

/// NetworkHygiene provides lightweight, stateful checks for network hygiene
/// such as deduplication and rate limiting.
// These checks may mutate internal counters or caches and are not strictly read-only.
type NetworkHygiene interface {
	UpdateAndCheckRateLimit(peerID int) bool
	IsDuplicateMessage(peerID int, seq uint64) bool
}

/// StateReader provides a read-only view into the ledger state.
/// It allows components to query consensus progress and object availability
/// without mutating the underlying state machine.
type StateReader interface {
	CurrentRound() int
	IsKnownVertex(hash Hash) bool
	IsFinalized(hash Hash) bool
}

/// SyncState tracks the lifecycle of asynchronous data synchronization.
type SyncState interface {
	IsRequestPending(hash Hash) bool
}

// ConsensusContext provides the runtime coordination surface required by
// consensus components.
//
// It exposes:
// - local state visibility (rounds, known vertices, finalization)
// - synchronization status (pending fetch lifecycle)
// - proposal lifecycle control
// - quorum aggregation
// - signing authority
//
// This interface is typically implemented by the Replica orchestration layer,
// not by stateless validation components.
type ConsensusContext interface {
	NetworkHygiene
	StateReader
	SyncState
	ID() int
	CheckQuorum(QCType) ([]*Message, int)
	AlreadyProposed(round int) bool
	MarkAsProposed(round int)
	Sign(data []byte) Signature
}
