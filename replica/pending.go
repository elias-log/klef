// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Pending request accessors provide lifecycle control for asynchronous vertex retrieval.

Key properties:
- Request Tracking: Exposes controlled access to the PendingManager.
- Deduplication: Prevents redundant fetch requests for the same missing vertex.
- Availability Support: Helps maintain DAG completeness by tracking unresolved dependencies.

Note:
- These methods act as a coordination bridge between Replica orchestration
  and the underlying PendingManager state.
- Pending state is local and ephemeral; it does not affect consensus or DAG correctness.
*/

package replica

import "klef/pkg/types"

/// IsRequestPending reports whether a vertex hash is currently under active retrieval.
/// Used to suppress duplicate network requests for the same missing dependency.
func (r *Replica) IsRequestPending(hash types.Hash) bool {
	return r.pendingMgr.IsPending(hash)
}

/// AddPendingRequest registers a vertex hash as an active fetch target.
/// Duplicate registrations are expected to be idempotent at the PendingManager layer.
func (r *Replica) AddPendingRequest(hash types.Hash) bool {
	return r.pendingMgr.Add(hash)
}

/// RemovePendingRequest clears a completed or abandoned fetch lifecycle entry.
/// Typically called after successful DAG integration or terminal fetch failure.
func (r *Replica) RemovePendingRequest(hash types.Hash) {
	r.pendingMgr.Remove(hash)
}

func (r *Replica) WatchRequests(hashes []types.Hash) <-chan struct{} {
	return r.pendingMgr.Watch(hashes)
}

func (r *Replica) GetMissingHashes(hashes []types.Hash) []types.Hash {
	return r.dag.GetMissingHashes(hashes)
}
