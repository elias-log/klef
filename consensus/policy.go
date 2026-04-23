// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package consensus

import (
	"klef/types"
	"math"
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
	globalRatio     float64
	committeeRatio  float64
}

func NewDynamicQuorumPolicy(totalN int, committeeN int, gRatio, cRatio float64) *DynamicQuorumPolicy {
	return &DynamicQuorumPolicy{
		oldTotalN:       totalN,
		newTotalN:       totalN,
		oldCommitteeN:   committeeN,
		newCommitteeN:   committeeN,
		isTransitioning: false,
		globalRatio:     gRatio,
		committeeRatio:  cRatio,
	}
}

// GetQuorumSize: QC 타입에 따라 정족수를 계산하네!
func (p *DynamicQuorumPolicy) GetQuorumSize(qcType types.QCType) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var ratio float64
	var oldN, newN int

	if qcType == types.QCGlobal {
		ratio = p.globalRatio
		oldN, newN = p.oldTotalN, p.newTotalN
	} else {
		ratio = p.committeeRatio
		oldN, newN = p.oldCommitteeN, p.newCommitteeN
	}

	calc := func(n int, r float64) int {
		return int(math.Floor(float64(n)*r)) + 1
	}

	if !p.isTransitioning {
		return calc(newN, ratio)
	}

	// 변환기 보수적 전략
	oldQ := calc(oldN, ratio)
	newQ := calc(newN, ratio)

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
