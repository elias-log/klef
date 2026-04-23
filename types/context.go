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
	GetCurrentRound() int
	IsKnownVertex(hash Hash) bool
	IsFinalized(hash Hash) bool
}

/// SyncState tracks the lifecycle of asynchronous data synchronization.
type SyncState interface {
	IsRequestPending(hash Hash) bool
}

/// ConsensusContext extends StateReader with write-access and signing capabilities.
// - This interface provides the active consensus engine (e.g., Proposer, Validator)
// with the tools necessary to participate in the consensus protocol,
// manage proposal lifecycles, and produce cryptographic proofs.
// - It represents the only authority through which consensus modules
// are allowed to mutate protocol state or emit signed artifacts.
type ConsensusContext interface {
	NetworkHygiene
	StateReader
	SyncState
	GetID() int
	GetReadyQuorum() ([]*Message, int)
	AlreadyProposed(round int) bool
	MarkAsProposed(round int)
	Sign(data []byte) Signature
}
