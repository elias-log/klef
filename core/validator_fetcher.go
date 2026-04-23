// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator Pending Request Interface provides lifecycle control of asynchronous data requests.

Key properties:
- Request Tracking: Interface for querying and updating the 'PendingManager'.
- State Ownership: Centralizes pending request state within the Validator.
- Data Availability: Supports the 'Fetcher' component in resolving causal gaps in the DAG.

Note:
- These methods act as a bridge between the high-level Validator and the
  low-level PendingRequest ledger.
*/

package core

import "klef/types"

/// IsRequestPending checks if a specific vertex hash is currently under
/// active retrieval from the network.
/// Used to avoid duplicate network requests and reduce redundant traffic.
func (v *Validator) IsRequestPending(hash types.Hash) bool {
	return v.pendingMgr.IsPending(hash)
}

/// AddPendingRequest registers a hash into the tracking ledger to monitor its
/// synchronization status.
/// Assumes idempotent behavior; duplicate insertions should be safely ignored by PendingManager.
func (v *Validator) AddPendingRequest(hash types.Hash) {
	v.pendingMgr.Add(hash)
}

/// RemovePendingRequest marks the completion of a fetch lifecycle,
/// typically after successful DAG integration.
func (v *Validator) RemovePendingRequest(hash types.Hash) {
	v.pendingMgr.Remove(hash)
}
