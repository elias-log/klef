// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
PendingManager manages request deduplication, retry scheduling,
and lifecycle tracking for outbound synchronization requests.

Key properties:
- Single-flight Guarantee: Ensures that at most one active request
  per hash exists within a valid time window.
- Backoff & Jitter: Prevents network congestion using exponential backoff
  (bounded by MaxBackoff) combined with randomized jitter.
- Expiry-ordered Cleanup: Uses a min-heap keyed by expiration time
  to efficiently purge stale requests.
- Lazy Deletion: Remove operations update only the map,
  while stale heap entries are discarded during cleanup.
  Correctness is preserved by validating entries against the current map state.
- Heap/Map Decoupling: The priority queue may contain stale entries;
  the map acts as the source of truth.
- Localized RNG: Uses a dedicated PRNG instance to avoid potential
  contention on shared random sources.

Note:
- The manager ensures that the system does not redundantly request the same
  vertex while a valid request is still in flight.
*/

package core

import (
	"arachnet-bft/config"
	"arachnet-bft/internal/ds"
	"container/heap"
	"context"
	"math/rand"
	"sync"
	"time"
)

const (
	minCleanupInterval = 100 * time.Millisecond
	maxCleanupInterval = 500 * time.Millisecond
)

/// PendingItem represents a request entry within the Priority Queue.
type PendingItem struct {
	Hash        string
	RequestTime time.Time
	ExpiryTime  time.Time
	index       int
}

/// PendingMeta tracks detailed request telemetry for backoff calculations.
type PendingMeta struct {
	RequestTime time.Time
	ExpiryTime  time.Time
	RetryCount  int
}

/// PendingManager coordinates the timing and persistence of pending data fetches.
type PendingManager struct {
	mu              sync.RWMutex
	pendingRequests map[string]PendingMeta
	pendingQueue    ds.PriorityQueue
	cfg             *config.Config
	rng             *rand.Rand
}

/// NewPendingManager initializes a manager with a locally seeded PRNG.
func NewPendingManager(cfg *config.Config) *PendingManager {
	return &PendingManager{
		pendingRequests: make(map[string]PendingMeta),
		pendingQueue:    make(ds.PriorityQueue, 0),
		cfg:             cfg,
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

/// Add registers a new fetch request or updates an existing one with incremented backoff.
func (pm *PendingManager) Add(hash string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	meta, exists := pm.pendingRequests[hash]

	// 1. Redundancy Guard: Ignore if an active request for the same hash is still valid.
	if exists && now.Before(meta.ExpiryTime) {
		return
	}

	// 2. Backoff Calculation: Exponentially increase wait time up to 15 retries.
	retryCount := 0
	if exists {
		retryCount = meta.RetryCount + 1
	}
	if retryCount > 15 {
		retryCount = 15
	}
	backoff := pm.cfg.Request.BaseTimeout * time.Duration(1<<retryCount)
	if backoff > pm.cfg.Request.MaxBackoff {
		backoff = pm.cfg.Request.MaxBackoff
	}

	// 3. Jitter Injection: Local RNG usage prevents global seed contention.
	jitter := time.Duration(0)
	if backoff > 0 {
		jitter = time.Duration(pm.rng.Int63n(int64(backoff / 5)))
	}

	expiry := now.Add(backoff + jitter)

	// 4. Persistence: Update mapping and priority heap.
	pm.pendingRequests[hash] = PendingMeta{
		RequestTime: now,
		ExpiryTime:  expiry,
		RetryCount:  retryCount,
	}

	heap.Push(&pm.pendingQueue, &PendingItem{
		Hash:        hash,
		RequestTime: now,
		ExpiryTime:  expiry,
	})
}

/// Remove purges a hash from the active tracking map.
/// Causal cleanup in the Priority Queue will be handled lazily by the background loop.
func (pm *PendingManager) Remove(hash string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.pendingRequests, hash)
}

/// IsPending checks if a hash is currently awaiting a response within its valid window.
func (pm *PendingManager) IsPending(hash string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	meta, exists := pm.pendingRequests[hash]
	return exists && time.Now().Before(meta.ExpiryTime)
}

/// StartCleanupLoop initiates the periodic eviction of stale requests.
func (pm *PendingManager) StartCleanupLoop(ctx context.Context) {
	interval := pm.getCleanupInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.cleanup()
		case <-ctx.Done():
			return
		}
	}
}

/// cleanup performs a batch removal of expired or invalidated items from the Priority Queue.
func (pm *PendingManager) cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	for pm.pendingQueue.Len() > 0 {
		item := pm.pendingQueue[0].(*PendingItem)
		meta, exists := pm.pendingRequests[item.Hash]

		// Integrity Check: Pop if the item was deleted via Remove() or replaced by a new Add().
		if !exists || !meta.RequestTime.Equal(item.RequestTime) || !meta.ExpiryTime.Equal(item.ExpiryTime) {
			heap.Pop(&pm.pendingQueue)
			continue
		}

		// Min-Heap property: If the earliest item is still valid, all subsequent items are too.
		// Since the heap is ordered by ExpiryTime, all subsequent items
		// must have equal or later expiry times.
		if now.Before(item.ExpiryTime) {
			break
		}

		heap.Pop(&pm.pendingQueue)
		delete(pm.pendingRequests, item.Hash)
	}
}

/// getCleanupInterval dynamically calculates the background GC (Garbage Collection) frequency.
/// It ensures the interval scales with the request timeout while remaining within
/// safe operational bounds.
func (pm *PendingManager) getCleanupInterval() time.Duration {
	// Target 25% of the base timeout to ensure stale requests are purged promptly.
	interval := pm.cfg.Request.BaseTimeout / 4

	// Clamp the interval to maintain a balance between CPU usage and memory efficiency.
	if interval < minCleanupInterval {
		return minCleanupInterval
	}
	if interval > maxCleanupInterval {
		return maxCleanupInterval
	}
	return interval
}

// Internal Priority Queue Interface implementations
func (pi *PendingItem) Less(other ds.HeapItem) bool {
	return pi.ExpiryTime.Before(other.(*PendingItem).ExpiryTime)
}
func (pi *PendingItem) SetIndex(idx int) { pi.index = idx }
func (pi *PendingItem) GetIndex() int    { return pi.index }
