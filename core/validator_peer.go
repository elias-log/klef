// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Validator Peer Management extensions handle remote node discovery and liveness.

Key properties:
- Concurrency Control: Uses Read-Write Mutexes to balance high-frequency peer
  queries and occasional state updates.
- Peer Round Tracking: Maintains a view of the network's logical height to
  optimize data fetching strategies.
- Non-deterministic Sampling: Implements randomized peer selection for gossip
  protocols and request load balancing.
- Decoupling: Separates peer state management from core consensus logic.

Note:
- This logic is a candidate for migration to a standalone 'PeerManager'
  during the Phase 2 refactoring (Orchestrator integration).
*/

package core

import (
	"math/rand"
	"time"
)

/// AddPeer registers a new remote node into the active validator set.
func (v *Validator) AddPeer(id int) {
	v.peersMu.Lock()
	defer v.peersMu.Unlock()
	v.Peers[id] = true
}

/// UpdatePeerRound updates the observed logical clock height for a specific peer.
/// This information is used to prioritize targets during the Vertex synchronization process.
func (v *Validator) UpdatePeerRound(peerID int, round int) {
	v.peersMu.Lock()
	defer v.peersMu.Unlock()

	if round > v.PeerRounds[peerID] {
		v.PeerRounds[peerID] = round
	}
}

/// getActivePeerIDs returns a snapshot of currently connected and active node IDs.
func (v *Validator) getActivePeerIDs() []int {
	v.peersMu.RLock()
	defer v.peersMu.RUnlock()

	ids := make([]int, 0, len(v.Peers))
	for id, active := range v.Peers {
		if active {
			ids = append(ids, id)
		}
	}
	return ids
}

/// GetRandomPeers selects n distinct active peers for gossip dissemination or data requests.
/// If n exceeds the total active count, all available active peers are returned.
func (v *Validator) GetRandomPeers(n int) []int {
	allPeers := v.getActivePeerIDs()
	totalActive := len(allPeers)

	if totalActive == 0 {
		return []int{}
	}

	if totalActive <= n {
		return allPeers
	}

	return v.shuffleAndPick(allPeers, n)
}

/// shuffleAndPick performs an in-place Fisher-Yates shuffle on a slice snapshot
/// using a locally seeded PRNG (Pseudo-Random Number Generator).
func (v *Validator) shuffleAndPick(peers []int, n int) []int {
	// Safe to mutate as 'peers' is a fresh slice from getActivePeerIDs.
	// Use a local source to avoid global lock contention on the default rand instance.

	// TODO(Optimization): If called at high frequency, consider pre-allocating
	// a PRNG per Validator to reduce seed generation overhead (time.Now().UnixNano()).

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	return peers[:n]
}
