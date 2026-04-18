package core

import (
	"arachnet-bft/types"
	"sync"
)

type VotePool struct {
	mu    sync.RWMutex
	votes map[int][]*types.Message // Round -> Votes
}

func NewVotePool() *VotePool {
	return &VotePool{
		votes: make(map[int][]*types.Message),
	}
}

func (vp *VotePool) GetVotesByRound(round int) []*types.Message {
	vp.mu.RLock()
	defer vp.mu.RUnlock()
	return vp.votes[round]
}

// AddVote 등 투표를 채워넣는 로직도 여기에 추가할 것이네.
