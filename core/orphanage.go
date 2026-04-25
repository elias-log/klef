// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Orphanage manages vertices with missing causal links (parents).

Key properties:
- Invariant Consistency: The 'lostParentCount' and 'orphans' maps must remain synchronized.
- Zero-Count Liberation: Vertices move to 'ready' status immediately upon causal resolution.
- Deterministic Eviction: Uses 'Missing Count' and 'Hash Lexicography' to handle overflow.

Design Principle:
	Non-determinism is tolerated at the buffering layer,
	but must be eliminated before DAG insertion.

Network (nondeterministic)
    ↓
Orphanage (partially nondeterministic)
    ↓
DAG insertion (deterministic)
    ↓
Finalizer (deterministic)

Limitations & Local Determinism:
    - Localized Eviction: findVictim relies on the locally observed 'missingCount'.
      Therefore, if nodes observe parent availability at different times,
      the eviction target may differ across nodes even under the same capacity constraints.
    - Convergence Principle: The non-determinism in eviction is transient.
      Once missing parent data is eventually received,
      all nodes converge to an identical final DAG structure.
    - Trade-off: Prioritizes orphanage turnover efficiency based on 'Missing Count'
      over global determinism based on 'Total Parent Count'.

- Convergence Guarantee (Assumption):
	The convergence to an identical DAG is guaranteed under the assumption of eventual data delivery
	(i.e., all missing parent vertices are eventually received) and no permanent data loss.

- Non-Consensus Scope:
    Orphanage is not consensus-critical as long as:
    (1) all nodes eventually receive the same set of vertices, and
    (2) released vertices are deterministically ordered before DAG insertion.
*/

package core

import (
	"klef/pkg/types"
	"sync"
)

/// Orphanage acts as a temporary buffer for vertices awaiting their ancestors.
type Orphanage struct {
	mu              sync.Mutex
	lostParents     map[types.Hash][]*types.Vertex // ParentHash -> Waiting Children
	lostParentCount map[types.Hash]int             // ChildHash -> Number of missing parents
	orphans         map[types.Hash]*types.Vertex   // ChildHash -> Vertex Object
	capacity        int                            // Maximum number of orphans allowed
}

/// NewOrphanage initializes a buffer with a fixed capacity to prevent memory exhaustion.
func NewOrphanage(limit int) *Orphanage {
	return &Orphanage{
		lostParents:     make(map[types.Hash][]*types.Vertex),
		lostParentCount: make(map[types.Hash]int),
		orphans:         make(map[types.Hash]*types.Vertex),
		capacity:        limit,
	}
}

/// AddOrphan registers a vertex in the buffer. If capacity is exceeded,
/// it deterministically evicts a 'victim' based on the missing parent count.
func (b *Orphanage) AddOrphan(vtx *types.Vertex, missingParents []types.Hash) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 1. Idempotency: Prevent duplicate entries.
	if _, exists := b.lostParentCount[vtx.Hash]; exists {
		return
	}

	// 2. Capacity Management: Evict the least likely to be resolved if full.
	missingCount := len(missingParents)
	if len(b.lostParentCount) >= b.capacity {
		victim := b.findVictim(vtx.Hash, missingCount)
		if victim == vtx.Hash {
			return
		}
		b.removeOrphan(victim)
	}

	// 3. Dependency Registration.
	for _, h := range missingParents {
		b.lostParents[h] = append(b.lostParents[h], vtx)
	}
	b.orphans[vtx.Hash] = vtx
	b.lostParentCount[vtx.Hash] = missingCount
}

/// findVictim identifies the best candidate for eviction.
// Eviction Strategy:
// - Prefer evicting vertices with higher missing parent count
//   (lower likelihood of near-term resolution)
// - Break ties deterministically via lexicographic hash ordering
func (b *Orphanage) findVictim(candidateHash types.Hash, candidateMissingCount int) types.Hash {
	// Include the incoming candidate in eviction comparison.
	// This ensures that insertion is rejected if it is less favorable than existing entries.
	victimHash := candidateHash
	maxMissing := candidateMissingCount

	for h, count := range b.lostParentCount {
		// 1. Higher missing count
		if count > maxMissing {
			victimHash = h
			maxMissing = count
		} else if count == maxMissing {
			// 2. Larger hash (lexicographical tie-break).
			if h.Compare(victimHash) > 0 {
				victimHash = h
			}
		}
	}
	return victimHash
}

/// OnParentArrival resolves causal dependencies for children waiting on pHash.
/// Returns a deterministically sorted list of vertices ready for DAG integration.
func (b *Orphanage) OnParentArrival(pHash types.Hash) []*types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()

	children, ok := b.lostParents[pHash]
	if !ok {
		return nil
	}

	// Canonical Sort:
	// Required because insertion order into lostParents is non-deterministic
	// across nodes (depends on message arrival timing).
	types.SortVertices(children)

	var readyChildren []*types.Vertex
	for _, child := range children {
		count, exists := b.lostParentCount[child.Hash]
		if !exists {
			continue
		}

		newCount := count - 1
		if newCount <= 0 {
			readyChildren = append(readyChildren, child)
			delete(b.lostParentCount, child.Hash)
			delete(b.orphans, child.Hash)
		} else {
			b.lostParentCount[child.Hash] = newCount
		}
	}

	// Double-sorting for the output to ensure deterministic DAG insertion order.
	types.SortVertices(readyChildren)

	// Safe to delete:
	// This parent dependency has been fully resolved for all waiting children.
	delete(b.lostParents, pHash)
	return readyChildren
}

/// removeOrphan purges a vertex and all its dependency mappings from the buffer.
// NOTE:
// This is O(n) per parent. Acceptable for current scale.
// Future optimization: index children by hash for O(1) removal.
func (b *Orphanage) removeOrphan(hash types.Hash) {
	vtx, exists := b.orphans[hash]
	if !exists {
		return
	}

	for _, pHash := range vtx.Parents {
		if list, ok := b.lostParents[pHash]; ok {
			for i, child := range list {
				if child.Hash == hash {
					b.lostParents[pHash] = append(list[:i], list[i+1:]...)
					break
				}
			}
			if len(b.lostParents[pHash]) == 0 {
				delete(b.lostParents, pHash)
			}
		}
	}

	delete(b.lostParentCount, hash)
	delete(b.orphans, hash)
}

func (b *Orphanage) GetOrphan(hash types.Hash) *types.Vertex {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.orphans[hash]
}

func (o *Orphanage) Size() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.orphans)
}
