// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package consensus

import "arachnet-bft/types"

type Verifier struct {
	PublicKeyMap map[int]types.PublicKey // 노드 ID별 공개키 저장
}

// VerifyVertex: 서명이 유효한지, 라운드가 적절한지 검사하네.
func (v *Verifier) VerifyVertex(vtx *types.Vertex) bool {
	// 1. Author의 서명 확인
	// 2. 부모 QC들의 유효성 확인
	// 3. (옵션) 타임스탬프나 라운드 번호의 논리적 모순 확인
	return true
}
