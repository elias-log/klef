package consensus

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"sync"
)

// QuorumPolicy: 정족수 판단을 위한 범용 인터페이스일세.
type QuorumPolicy interface {
	// IsQuorum: 현재 모인 부모(또는 투표)들의 목록이 합의에 충분한지 판단하네.
	// 미래에 Jolteon이나 동적 멤버십 로직이 여기로 들어올 걸세.
	IsQuorum(qc *types.QC) bool

	// GetQuorumSize: 현재 정책상 필요한 최소 숫자를 반환하네.
	GetQuorumSize(qcType types.QCType) int
}

// DynamicQuorumPolicy: '동적 N' 대응용 정책일세.
type DynamicQuorumPolicy struct {
	mu sync.RWMutex
	// 글로벌 정족수용 N
	oldTotalN int
	newTotalN int
	// 커미티 정족수용 N
	oldCommitteeN   int
	newCommitteeN   int
	isTransitioning bool // 지금 멤버십이 변하는 중인가?
	Config          *config.Config
}

func NewDynamicQuorumPolicy(totalN int, committeeN int, cfg *config.Config) *DynamicQuorumPolicy {
	return &DynamicQuorumPolicy{
		oldTotalN:       totalN,
		newTotalN:       totalN,
		oldCommitteeN:   committeeN,
		newCommitteeN:   committeeN,
		isTransitioning: false,
		Config:          cfg,
	}
}

// GetQuorumSize: 이제 QC 타입에 따라 정족수를 계산하네!
func (p *DynamicQuorumPolicy) GetQuorumSize(qcType types.QCType) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var num, den, oldN, newN int

	// 1. 타입에 따른 분자/분모 및 N 선택
	if qcType == types.QCGlobal {
		num = p.Config.GlobalQuorumNumerator
		den = p.Config.GlobalQuorumDenominator
		oldN = p.oldTotalN
		newN = p.newTotalN
	} else {
		num = p.Config.CommitteeQuorumNumerator
		den = p.Config.CommitteeQuorumDenominator
		oldN = p.oldCommitteeN
		newN = p.newCommitteeN
	}

	// 2. 평상시 계산 (n * num / den + 1)
	if !p.isTransitioning {
		return (newN * num / den) + 1
	}

	// 3. 변환기 보수적 전략
	oldQ := (oldN * num / den) + 1
	newQ := (newN * num / den) + 1

	if oldQ > newQ {
		return oldQ
	}
	return newQ
}

func (p *DynamicQuorumPolicy) IsQuorum(qc *types.QC) bool {
	// 1. 필요한 정족수 크기를 가져오고
	required := p.GetQuorumSize(qc.Type)

	// 2. 현재 서명 개수가 그보다 크거나 같은지 확인하네.
	return len(qc.Signatures) >= required
}
