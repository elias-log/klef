package core

import "math/rand"

// AddPeer: 새로운 피어가 연결되었을 때 불러주게나.
func (v *Validator) AddPeer(id int) {
	v.peersMu.Lock()
	defer v.peersMu.Unlock()
	v.Peers[id] = true
}

// UpdatePeerRound: 특정 피어가 알려준 최신 라운드 정보를 갱신하네.
// 나중에 따라잡기(Sync) 로직에서 누구에게 데이터를 요청할지 결정하는 기준이 되지.
func (v *Validator) UpdatePeerRound(peerID int, round int) {
	v.peersMu.Lock()
	defer v.peersMu.Unlock()

	if round > v.PeerRounds[peerID] {
		v.PeerRounds[peerID] = round
	}
}

// getActivePeerIDs: 현재 살아있는 피어들의 ID 리스트를 스냅샷으로 가져오네.
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

// GetRandomPeers: Gossip이나 데이터 요청을 위해 무작위로 n개의 피어를 선정하네.
func (v *Validator) GetRandomPeers(n int) []int {
	allPeers := v.getActivePeerIDs()
	totalActive := len(allPeers)

	if totalActive == 0 {
		return []int{}
	}

	// 요청한 n보다 활성 노드가 적거나 같으면 전체 반환
	if totalActive <= n {
		return allPeers
	}

	// 노드가 충분하다면, 그중 n개만 무작위로 추출하세.
	return v.shuffleAndPick(allPeers, n)
}

func (v *Validator) shuffleAndPick(peers []int, n int) []int {
	// 원본 peers는 getActivePeerIDs에서 새로 만든 슬라이스이므로 바로 섞어도 안전하네.
	rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	return peers[:n]
}
