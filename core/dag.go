// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
DAG (Directed Acyclic Graph) serves as the core ledger of Klef.

Key properties:
- Deterministic insertion via canonical ordering and sorted orphan release
- Worklist-based non-recursive orphan resolution (prevents stack overflow)
- Lazy round indexing for performance efficiency
- Multi-layered validation (hash, structure, slashing)

Note:
[KNOWN LIMITATIONS]
- No full semantic validation (round / equivocation)
- Locking is not strictly fine-grained
- Parent ordering must remain deterministic (critical invariant)
- Local non-determinism in orphan buffering is permitted,
  but does not affect final outcome determinism due to
  deterministic DAG insertion and conflict resolution.

[LOCK ORDER GUARANTEE]
- Hierarchy: DAG.mu > Orphanage.mu
- Invariant: The Orphanage must NEVER call back into the DAG to prevent deadlocks.
*/

package core

import (
	"fmt"
	"klef/config"
	"klef/types"
	"sort"
	"sync"
)

/// SyncFetcher defines the interface for triggering external data retrieval.
type SyncFetcher interface {
	StartSync(missingHashes []types.Hash, suspectID int)
}

/// DAG manages the causal history of vertices and their quorum-based finality.
type DAG struct {
	mu         sync.RWMutex
	Vertices   map[types.Hash]*types.Vertex // Global registry: Hash -> Vertex
	RoundIndex map[int][]types.Hash         // Round-based lookup: Round -> []Hashes
	Buffer     *Orphanage                   // Buffer for vertices with missing causal links
	Slasher    *Slasher                     // Penalizes protocol violations
	Fetcher    SyncFetcher                  // Triggers sync for missing ancestors
	Config     *config.Config               // Protocol parameters
}

/// NewDAG initializes the DAG engine and its associated orphan buffer.
func NewDAG(fetcher SyncFetcher, slasher *Slasher, cfg *config.Config) *DAG {
	orphanage := NewOrphanage(cfg.DAG.OrphanCapacity)

	return &DAG{
		Vertices:   make(map[types.Hash]*types.Vertex),
		RoundIndex: make(map[int][]types.Hash),
		Buffer:     orphanage,
		Slasher:    slasher,
		Fetcher:    fetcher,
		Config:     cfg,
	}
}

/// AddVertex attempts to integrate a new vertex into the DAG.
/// It performs integrity checks, causal validation, and triggers orphan resolution.
func (d *DAG) AddVertex(vtx *types.Vertex, currentNodeRound int) {
	// Phase 0: Optimistic duplicate check.
	d.mu.RLock()
	_, exists := d.Vertices[vtx.Hash]
	d.mu.RUnlock()
	if exists {
		return
	}

	// Phase 1: Cryptographic Integrity Verification.
	calcHash := vtx.CalculateHash()
	if vtx.Hash != calcHash {
		fmt.Printf("[ERROR] DAG: Hash mismatch detected for vertex %s\n", vtx.Hash)
		// TODO(Safety): Implement 'Challenge-Response' or immediate rejection.
		return
	}

	// Phase 2: Protocol Malformation Check (Byzantine Defense).
	if isMalformed, reason := types.CheckMalformed(vtx.Parents); isMalformed {
		fmt.Printf("[CRITICAL] Byzantine node %d detected: %s\n", vtx.Author, reason)
		d.Slasher.AddDemerit(
			vtx.Author,
			d.Config.Security.MalformedVertexPenalty,
			vtx,
			reason,
		)
		return
	}

	// Phase 3: Causal Link Validation (Lock Acquisition).
	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check after lock acquisition to prevent race conditions.
	if _, exists := d.Vertices[vtx.Hash]; exists {
		return
	}

	missing := d.getMissingHashesLocked(vtx.Parents)

	// Phase 4: Orphan Handling & Sync Trigger.
	if len(missing) > 0 {
		fmt.Printf("[DEBUG] DAG: Vertex %s stored in Orphanage (Missing: %d)\n", vtx.Hash.String()[:8], len(missing))
		d.Buffer.AddOrphan(vtx, missing)

		// Sync logic: Triggered if the vertex round is significantly ahead.
		diff := vtx.Round - currentNodeRound
		if diff > d.Config.DAG.SyncTriggerThreshold {
			d.Fetcher.StartSync(missing, vtx.Author)
		}
		return
	}

	// TODO: validateVertex(vtx)
	// - Round monotonicity
	// - QC consistency
	// - Author equivocation detection

	// Phase 5: Official Insertion & Recursive Cascade.
	d.processInsertionLocked(vtx)
}

// Deterministic Sort: Critical invariant.
// Ensures identical orphan resolution order across all nodes,
// which is essential for global outcome determinism.
func (d *DAG) processInsertionLocked(initialVtx *types.Vertex) {
	worklist := []*types.Vertex{initialVtx}
	affectedRounds := make(map[int]bool)

	for len(worklist) > 0 {
		vtx := worklist[0]
		worklist = worklist[1:]

		if _, exists := d.Vertices[vtx.Hash]; exists {
			continue
		}

		// Step A: Persist vertex data and index by round.
		d.Vertices[vtx.Hash] = vtx
		d.RoundIndex[vtx.Round] = append(d.RoundIndex[vtx.Round], vtx.Hash)
		affectedRounds[vtx.Round] = true // Lazy Sorting

		// Step B: Resolve orphans that were waiting for this vertex.
		readyChildren := d.Buffer.OnParentArrival(vtx.Hash)
		if len(readyChildren) > 0 {
			// Deterministic Sort: Essential for maintaining identical DAG traversal across nodes.
			sort.Slice(readyChildren, func(i, j int) bool {
				return readyChildren[i].Hash.Compare(readyChildren[j].Hash) < 0
			})
			worklist = append(worklist, readyChildren...)
		}

		fmt.Printf("[DEBUG] DAG: Vertex %s committed\n", vtx.Hash.String()[:8])
	}
	// Phase 6: Canonical Index Normalization.
	// Only sort the affected rounds to maintain performance.
	rounds := make([]int, 0, len(affectedRounds))
	for r := range affectedRounds {
		rounds = append(rounds, r)
	}
	sort.Ints(rounds)

	for _, r := range rounds {
		types.SortHashes(d.RoundIndex[r])
	}
}

/// getMissingHashesLocked identifies ancestors not yet present in the DAG.
// Must be called while holding DAG.mu.
func (d *DAG) getMissingHashesLocked(hashes []types.Hash) []types.Hash {
	var missing []types.Hash
	for _, h := range hashes {
		if _, exists := d.Vertices[h]; !exists {
			missing = append(missing, h)
		}
	}
	return missing
}

/// GetVertex retrieves a vertex by its unique hash identifier.
func (d *DAG) GetMissingHashes(hashes []types.Hash) []types.Hash {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var missing []types.Hash
	for _, h := range hashes {
		if _, exists := d.Vertices[h]; !exists {
			missing = append(missing, h)
		}
	}
	return missing
}

/// GetVerticesByRound returns all vertices committed at a specific logical height.
func (d *DAG) GetVertex(hash types.Hash) *types.Vertex {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.Vertices[hash]
}

/// GetVerticesByRound returns all vertices committed at a specific logical height.
func (d *DAG) GetVerticesByRound(round int) []*types.Vertex {
	d.mu.RLock()
	defer d.mu.RUnlock()

	hashes := d.RoundIndex[round]
	results := make([]*types.Vertex, 0, len(hashes))
	for _, h := range hashes {
		if vtx, exists := d.Vertices[h]; exists {
			results = append(results, vtx)
		}
	}
	return results
}

/// Size returns the total count of vertices officially integrated into the DAG.
func (d *DAG) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.Vertices)
}

/// GetTips identifies 'leaves' of the DAG—vertices that currently have no children.
/// These are potential parents for the next vertex proposal.
// NOTE:
// Current implementation performs a full scan.
// Future optimization: maintain an incremental tip index for O(1) updates.
func (d *DAG) GetTips() []*types.Vertex {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.Vertices) == 0 {
		return nil
	}

	// 1. Map all existing parent references.
	hasChild := make(map[types.Hash]bool)
	for _, vtx := range d.Vertices {
		for _, parentHash := range vtx.Parents {
			hasChild[parentHash] = true
		}
	}

	// 2. Vertices never referenced as a parent are the current 'Tips'.
	var tips []*types.Vertex
	for hash, vtx := range d.Vertices {
		if !hasChild[hash] {
			tips = append(tips, vtx)
		}
	}

	return tips
}
