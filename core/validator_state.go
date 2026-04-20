package core

import (
	"arachnet-bft/consensus"
	"arachnet-bft/types"
)

func (v *Validator) GetCurrentRound() int {
	return v.Round
}

func (v *Validator) IsKnownVertex(hash string) bool {
	return v.DAG.GetVertex(hash) != nil
}

func (v *Validator) Sign(data string) types.Signature {
	// string 해시를 바이트로 변환해서 서명 대리인에게 맡기네.
	return v.Signer.Sign([]byte(data))
}

func (v *Validator) GetQuorumPolicy() consensus.QuorumPolicy {
	return v.Policy
}

func (v *Validator) QuorumThreshold() int {
	return v.Policy.GetQuorumSize(types.QCGlobal)
}

func (v *Validator) GetID() int {
	return v.ID
}

// AlreadyProposed: 내가 이 라운드(혹은 그 이전)에 이미 제안했는지 확인하네.
func (v *Validator) AlreadyProposed(round int) bool {
	v.proposalMu.Lock()
	defer v.proposalMu.Unlock()
	return round <= v.lastProposedRound
}

// MarkAsProposed: 제안이 성공적으로 전파되었음을 장부에 기록하네.
func (v *Validator) MarkAsProposed(round int) {
	v.proposalMu.Lock()
	defer v.proposalMu.Unlock()
	if round > v.lastProposedRound {
		v.lastProposedRound = round
	}
}

// IsDuplicateMessage: 리플레이 공격 방어를 위한 체크 로직 (우선 Stub으로 구현하네)
func (v *Validator) IsDuplicateMessage(peerID int, seq uint64) bool {
	// TODO: Phase 4나 5 쯤에서 실제 장부(MessageHistory)를 확인하는 로직을 넣으세.
	return false
}

// UpdateAndCheckRateLimit: 노드별 요청 제한 (우선 Stub)
func (v *Validator) UpdateAndCheckRateLimit(peerID int) bool {
	// TODO: 특정 피어가 너무 많은 요청을 보내는지 체크하게나.
	return true // 일단은 모두 통과!
}

// IsFinalized: 특정 해시가 이미 글로벌 확정(Finalized)되었는지 확인
func (v *Validator) IsFinalized(hash string) bool {
	// TODO: DAG의 Anchor 포인트를 확인하는 로직이 들어갈 자리일세.
	return false
}
