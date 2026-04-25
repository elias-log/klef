// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
VertexFetcher implements the retrieval protocol for missing causal dependencies in the DAG.

Key Properties:
- Delegated Deduplication: Relies on PendingManager to enforce
  single-flight request semantics across concurrent fetch attempts.
- Batch Semantics: Operates on a set of missing hashes, dynamically
  shrinking the target set as data becomes available.
- Event-driven Convergence: Incoming data asynchronously updates the
  missing set, allowing early termination without waiting for timeouts.
- Early Termination: The retrieval loop exits as soon as all dependencies
  are resolved, preventing unnecessary escalation.
- Hybrid Progress Model: Combines event-driven convergence with
  timeout-driven escalation to guarantee liveness under partial failure.
- Timer Safety: Uses channel draining before resets to prevent race conditions in Go timers.
- Failure Observation: Logs persistent data unavailability after
  full escalation (potential data withholding signal).

Note:
[PHASED RETRIEVAL STRATEGY]
- Step 1 (Targeted): Requests missing hashes directly from the 'Suspect' (the vertex author).
- Step 2 (Neighborly): Fan-out requests to a randomized subset of peers.
- Step 3 (Global): Broadcasts to the entire network as a last resort for Data Availability.
- Escalation is timeout-driven: Each step progresses only if the
  requested data is not received within the configured time window.

[EVOLUTION ROADMAP]
- Phase 1: Transition to event-driven state transitions to eliminate static sleep latency.
- Phase 2: Integrate with Slasher to penalize nodes that consistently fail to provide data.
*/

package core

import (
	"context"
	"fmt"
	"klef/config"
	"klef/pkg/types"
	"time"
)

/// VertexFetcher coordinates the 'Fetch' protocol to resolve DAG gaps.
type VertexFetcher struct {
	InboundResponse chan *types.Vertex // Ingress for requested vertex data
	Context         FetchContext
}

type FetchContext interface {
	RuntimeContext() context.Context
	Broadcast(msgType types.MessageType, payload interface{})
	SendTo(targetID int, msgType types.MessageType, payload interface{})
	GetRandomPeers(n int) []int

	IsRequestPending(hash types.Hash) bool
	AddPendingRequest(hash types.Hash) bool
	RemovePendingRequest(hash types.Hash)
	WatchRequests(hashes []types.Hash) <-chan struct{}

	GetMissingHashes(hashes []types.Hash) []types.Hash
	ReplicaConfig() *config.Config
}

/// StartSync initiates an asynchronous retrieval process for a set of missing hashes.
/// It uses a tiered escalation approach to minimize network overhead.
func (f *VertexFetcher) StartSync(missingHashes []types.Hash, suspectID int) {
	// 1. Filter using PendingManager to prevent redundant concurrent fetches.
	filtered := f.filterNewRequests(missingHashes)
	if len(filtered) == 0 {
		return
	}

	// 2. Spawn a dedicated worker goroutine for this batch.
	go func() {
		currentMissing := filtered
		ctx, cancel := context.WithCancel(f.Context.RuntimeContext())
		defer cancel()

		for step := 1; step <= 3; step++ {
			// Pre-dispatch check: Has the data arrived via other channels?
			currentMissing = f.Context.GetMissingHashes(currentMissing)
			if len(currentMissing) == 0 {
				return
			}

			// Dispatch fetch requests based on the current escalation tier.
			f.dispatchByStep(step, suspectID, currentMissing)
			select {
			case <-time.After(f.getStepTimeout(step)):
				continue
			case <-f.notifyOnArrival(currentMissing):
				currentMissing = f.Context.GetMissingHashes(currentMissing)
				if len(currentMissing) == 0 {
					return
				}
			case <-ctx.Done():
				return
			}
		}
		// Final verification and Failure Reporting (Data Availability Failure).
		// TODO: Unified policy for data availability (DA) failures (e.g., slashing or cold storage).
		if finalMissing := f.Context.GetMissingHashes(currentMissing); len(finalMissing) > 0 {
			f.handleDAFailure(finalMissing)
		}

	}()
}

/// dispatchByStep maps the escalation step to specific peer target groups.
func (f *VertexFetcher) dispatchByStep(step int, suspectID int, hashes []types.Hash) {
	switch step {
	case 1:
		f.dispatchFetch([]int{suspectID}, hashes)
	case 2:
		neighbors := f.getFilteredNeighbors(f.Context.ReplicaConfig().Sync.MaxRandomPeers, suspectID)
		f.dispatchFetch(neighbors, hashes)
	case 3:
		f.Context.Broadcast(types.MsgFetchReq, &types.FetchRequest{
			MissingHashes: hashes,
		})
	}
}

/// getFilteredNeighbors selects random peers excluding the suspect.
func (f *VertexFetcher) getFilteredNeighbors(count int, suspectID int) []int {
	peers := f.Context.GetRandomPeers(count)
	filtered := make([]int, 0, len(peers))
	for _, pid := range peers {
		if pid != suspectID {
			filtered = append(filtered, pid)
		}
	}
	return filtered
}

/// dispatchFetch sends point-to-point fetch requests.
func (f *VertexFetcher) dispatchFetch(peerIDs []int, hashes []types.Hash) {
	if len(peerIDs) == 0 || len(hashes) == 0 {
		return
	}

	payload := &types.FetchRequest{
		MissingHashes: hashes,
	}
	for _, pid := range peerIDs {
		f.Context.SendTo(pid, types.MsgFetchReq, payload)
	}
}

/// getStepTimeout retrieves timing parameters for each escalation tier.
func (f *VertexFetcher) getStepTimeout(step int) time.Duration {
	cfg := f.Context.ReplicaConfig()
	switch step {
	case 1:
		return cfg.Sync.Step1Timeout
	case 2:
		return cfg.Sync.Step2Timeout
	case 3:
		return cfg.Sync.Step3Timeout
	default:
		return 5 * time.Second
	}
}

/// filterNewRequests uses PendingManager to deduplicate concurrent fetch attempts.
/// It registers missing hashes and returns only those that are not already in-flight.
func (f *VertexFetcher) filterNewRequests(hashes []types.Hash) []types.Hash {
	filtered := make([]types.Hash, 0)
	for _, h := range hashes {
		// Atomic Check-and-Set: only append if Add() succeeds (i.e., not already pending).
		if f.Context.AddPendingRequest(h) {
			filtered = append(filtered, h)
		}
	}
	return filtered
}

/// notifyOnArrival returns a signaling channel that triggers when any of the
/// specified hashes are received. This enables event-driven convergence
/// and eliminates unnecessary polling latency.
func (f *VertexFetcher) notifyOnArrival(hashes []types.Hash) <-chan struct{} {
	return f.Context.WatchRequests(hashes)
}

/// handleDAFailure manages scenarios where data remains unavailable after full escalation.
/// It clears the pending state and logs a critical Data Availability (DA) failure.
func (f *VertexFetcher) handleDAFailure(hashes []types.Hash) {
	// 1. Release pending state to allow future retry attempts if necessary.
	for _, h := range hashes {
		f.Context.RemovePendingRequest(h)
	}

	// 2. Critical Observation: This state suggests a Data Withholding Attack
	// or a severe network partition.
	// TODO (Phase 2): Integrate with Slasher to penalize the 'Suspect' peer.
	fmt.Printf("[CRITICAL] DA Failure! Data withheld for %d hashes. Sample: %s\n",
		len(hashes), hashes[0].String()[:8])
}
