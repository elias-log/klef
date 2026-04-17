package core

import "math/rand"

func (v *Validator) UpdatePeerRound(peerID int, round int) {
	if round > v.PeerRounds[peerID] {
		v.PeerRounds[peerID] = round
	}
}

// AddPeer: 새로운 피어가 연결되었을 때 불러주게나.
func (v *Validator) AddPeer(id int) {
	v.peersMu.Lock()
	defer v.peersMu.Unlock()
	v.Peers[id] = true
}

// getActivePeerIDs: 현재 살아있는 피어들의 ID만 슬라이스로 뽑아내네.
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

func (v *Validator) GetRandomPeers(n int) []int {
	// 1. 현재 연결된 모든 피어 리스트를 가져오네 (가정)
	allPeers := v.getActivePeerIDs()
	totalActive := len(allPeers)

	// 2. 요청한 n보다 활성 노드가 적거나 같으면,
	if totalActive <= n {
		// 더 고민할 것 없이 현재 있는 피어를 다 반환하면 되네.
		// 슬라이스를 복사해서 주는 게 안전하네 (원본 보호!)
		result := make([]int, totalActive)
		copy(result, allPeers)
		return result
	}

	// 3. 노드가 충분하다면, 그중 n개만 무작위로 추출하세.
	return v.shuffleAndPick(allPeers, n)
}

func (v *Validator) shuffleAndPick(peers []int, n int) []int {
	// 슬라이스 원본이 상하지 않게 복사본을 만듦세.
	shuffled := make([]int, len(peers))
	copy(shuffled, peers)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:n]
}
