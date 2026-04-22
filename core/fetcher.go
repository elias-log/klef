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
	"arachnet-bft/types"
	"fmt"
	"time"
)

/// VertexFetcher coordinates the 'Fetch' protocol to resolve DAG gaps.
type VertexFetcher struct {
	InboundResponse chan *types.Vertex // Ingress for requested vertex data
	Validator       *Validator         // Reference to parent for communication & config
}

/// StartSync initiates an asynchronous retrieval process for a set of missing hashes.
/// It uses a tiered escalation approach to minimize network overhead.
func (f *VertexFetcher) StartSync(missingHashes []string, suspectID int) {
	// 1. Filter using PendingManager to prevent redundant concurrent fetches.
	filtered := make([]string, 0)
	for _, h := range missingHashes {
		if !f.Validator.pendingMgr.IsPending(h) {
			f.Validator.pendingMgr.Add(h)
			filtered = append(filtered, h)
		}
	}
	if len(filtered) == 0 {
		return
	}

	// 2. Spawn a dedicated worker goroutine for this batch.
	go func() {
		currentMissing := filtered
		timer := time.NewTimer(time.Hour)
		if !timer.Stop() {
			<-timer.C
		} // Initialize in stopped state
		defer timer.Stop()

		for step := 1; step <= 3; step++ {
			// Pre-dispatch check: Has the data arrived via other channels?
			currentMissing = f.Validator.DAG.GetMissingHashes(currentMissing)
			if len(currentMissing) == 0 {
				return
			}

			// Dispatch fetch requests based on the current escalation tier.
			f.dispatchByStep(step, suspectID, currentMissing)
			// [Safety] Drain timer channel before reset to prevent premature firing.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(f.getStepTimeout(step))

		waitLoop:
			for {
				select {
				case <-f.InboundResponse:
					// Upon arrival, re-verify what is still missing.
					currentMissing = f.Validator.DAG.GetMissingHashes(currentMissing)
					if len(currentMissing) == 0 {
						return
					}

				case <-timer.C:
					// Timeout reached; escalate to the next tier of peers.
					break waitLoop

				case <-f.Validator.ctx.Done():
					return
				}
			}
		}

		// Step 4: Final verification and Failure Reporting (Data Availability Failure).
		// TODO: Unified policy for data availability (DA) failures (e.g., slashing or cold storage).
		finalMissing := f.Validator.DAG.GetMissingHashes(currentMissing)
		if len(finalMissing) > 0 {
			f.handlePanic(finalMissing)
		}
	}()
}

/// dispatchByStep maps the escalation step to specific peer target groups.
func (f *VertexFetcher) dispatchByStep(step int, suspectID int, hashes []string) {
	switch step {
	case 1:
		f.dispatchFetch([]int{suspectID}, hashes)
	case 2:
		neighbors := f.getFilteredNeighbors(f.Validator.Config.Sync.MaxRandomPeers, suspectID)
		f.dispatchFetch(neighbors, hashes)
	case 3:
		f.Validator.Broadcast(types.MsgFetchReq, &types.FetchRequest{
			MissingHashes: hashes,
		})
	}
}

/// getFilteredNeighbors selects random peers excluding the suspect.
func (f *VertexFetcher) getFilteredNeighbors(count int, suspectID int) []int {
	peers := f.Validator.GetRandomPeers(count)
	filtered := make([]int, 0, len(peers))
	for _, pid := range peers {
		if pid != suspectID {
			filtered = append(filtered, pid)
		}
	}
	return filtered
}

/// dispatchFetch sends point-to-point fetch requests.
func (f *VertexFetcher) dispatchFetch(peerIDs []int, hashes []string) {
	if len(peerIDs) == 0 || len(hashes) == 0 {
		return
	}

	payload := &types.FetchRequest{
		MissingHashes: hashes,
	}
	for _, pid := range peerIDs {
		f.Validator.SendTo(pid, types.MsgFetchReq, payload)
	}
}

/// getStepTimeout retrieves timing parameters for each escalation tier.
func (f *VertexFetcher) getStepTimeout(step int) time.Duration {
	switch step {
	case 1:
		return f.Validator.Config.Sync.Step1Timeout
	case 2:
		return f.Validator.Config.Sync.Step2Timeout
	case 3:
		return f.Validator.Config.Sync.Step3Timeout
	default:
		return 5 * time.Second
	}
}

/// handlePanic handles scenarios where data remains unavailable after full broadcast.
func (f *VertexFetcher) handlePanic(hashes []string) {
	// TODO(Safety): This indicates a potential data withholding attack or network partition.
	// Future: Downgrade 'Suspect' reputation and alert the Orchestrator.
	fmt.Printf("[CRITICAL] Data missing after full network broadcast: %v\n", hashes)
}
