// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package ds

/// HeapItem defines the interface for elements stored in the PriorityQueue.
/// It enables efficient index tracking for heap operations.
type HeapItem interface {
	Less(other HeapItem) bool // Returns true if this item should come before the other.
	SetIndex(idx int)         // Updates the current position of the item in the heap.
	GetIndex() int            // Retrieves the current position of the item.
}

/// PriorityQueue implements a standard heap-based priority queue for HeapItems.
/// It is designed to be used with the Go standard library's container/heap.
type PriorityQueue []HeapItem

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Less(pq[j]) }

/// Swap exchanges two elements and updates their internal index tracking.
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].SetIndex(i)
	pq[j].SetIndex(j)
}

/// Push adds an item to the priority queue.
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(HeapItem)
	item.SetIndex(n)
	*pq = append(*pq, item)
}

/// Pop removes and returns the highest priority item from the queue.
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.SetIndex(-1) // Invalidate index for safety
	old[n-1] = nil    // Avoid memory leaks (GC protection)
	*pq = old[0 : n-1]
	return item
}
