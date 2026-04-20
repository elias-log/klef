package core

import (
	"arachnet-bft/types"
)

// GetReadyQuorum: 투표 풀을 뒤져서 쿼럼(일단은 2f+1)이 형성된 라운드가 있는지 확인하네.
// 이 로직은 나중에 Jolteon 엔진이 '제안'을 할지 말지 결정하는 기준이 될 걸세.
func (v *Validator) GetReadyQuorum() ([]*types.Message, int) {
	// 현재 라운드 혹은 이전 라운드부터 뒤져보세 (보통 현재 라운드)
	round := v.Round
	votes := v.votePool.GetVotesByRound(round)

	policy := v.GetQuorumPolicy()
	requiredQuorum := policy.GetQuorumSize(types.QCGlobal) //일단은 2f+1

	if len(votes) >= requiredQuorum {
		return votes, round
	}

	return nil, 0
}
