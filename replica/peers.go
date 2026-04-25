// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Peer management accessors handle remote replica discovery, liveness tracking,
and peer sampling for network dissemination.

Key properties:
- Concurrency Control: Protects peer metadata with RWMutex.
- Peer Round Tracking: Maintains observed progress of remote replicas.
- Randomized Sampling: Supports gossip fanout and fetch distribution.
- Encapsulation: Isolates peer topology management from consensus execution.

Note:
- This logic may evolve into a dedicated PeerManager component
  as the orchestration layer becomes more modular.
*/

package replica

/// AddPeer registers a new remote node into the active validator set.
func (r *Replica) AddPeer(id int) {
	r.peersMu.Lock()
	defer r.peersMu.Unlock()
	r.peers[id] = true
}

/// UpdatePeerRound updates the observed logical clock height for a specific peer.
/// This information is used to prioritize targets during the Vertex synchronization process.
func (r *Replica) UpdatePeerRound(peerID int, round int) {
	r.peersMu.Lock()
	defer r.peersMu.Unlock()

	if round > r.peerRounds[peerID] {
		r.peerRounds[peerID] = round
	}
}

/// getActivePeerIDs returns a snapshot of currently connected and active node IDs.
func (r *Replica) getActivePeerIDs() []int {
	r.peersMu.RLock()
	defer r.peersMu.RUnlock()

	ids := make([]int, 0, len(r.peers))
	for id, active := range r.peers {
		if active {
			ids = append(ids, id)
		}
	}
	return ids
}

/// GetRandomPeers selects n distinct active peers for gossip dissemination or data requests.
/// If n exceeds the total active count, all available active peers are returned.
func (r *Replica) GetRandomPeers(n int) []int {
	allPeers := r.getActivePeerIDs()
	totalActive := len(allPeers)

	if totalActive == 0 {
		return []int{}
	}

	if totalActive <= n {
		return allPeers
	}

	return r.shuffleAndPick(allPeers, n)
}

/// shuffleAndPick performs an in-place Fisher-Yates shuffle on a slice snapshot
/// using a locally seeded PRNG (Pseudo-Random Number Generator).
func (r *Replica) shuffleAndPick(peers []int, n int) []int {
	if n > len(peers) {
		n = len(peers)
	}

	r.rng.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	return peers[:n]
}
