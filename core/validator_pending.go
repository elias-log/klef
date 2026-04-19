package core

import (
	"arachnet-bft/types"
)

// GetReadyQuorum: 어떤 라운드든 2f+1이 모인 게 있다면 (투표들, 그 라운드)를 반환하네.
func (v *Validator) GetReadyQuorum() ([]*types.Message, int) {
	// 1. 현재 라운드 혹은 이전 라운드부터 뒤져보세 (보통 현재 라운드)
	round := v.Round
	votes := v.votePool.GetVotesByRound(round)

	policy := v.GetQuorumPolicy()
	requiredQuorum := policy.GetQuorumSize(types.QCGlobal)

	if len(votes) >= requiredQuorum {
		return votes, round
	}

	// 만약 현재 라운드가 아니더라도, 혹시나 쿼럼이 형성된 라운드가 있는지 체크할 수도 있네.
	return nil, 0
}

func (v *Validator) IsRequestPending(hash string) bool {
	return v.pendingMgr.IsPending(hash)
}

func (v *Validator) AddPendingRequest(hash string) {
	v.pendingMgr.Add(hash)
}

func (v *Validator) RemovePendingRequest(hash string) {
	v.pendingMgr.Remove(hash)
}
