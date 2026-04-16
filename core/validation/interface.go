package validation

import "arachnet-bft/types"

// ValidatorContext는 검증에 필요한 최소한의 정보만 제공하는 인터페이스일세.
// 이를 통해 validation 패키지가 core 패키지를 직접 참조하지 않게 방어하네.
type ValidatorContext interface {
	GetCurrentRound() int
	IsKnownVertex(hash string) bool
	IsRequestPending(hash string) bool

	//TODO:
	//IsDuplicateMessage(peerID int, seq uint64) bool // Replay 방어
	//UpdateAndCheckRateLimit(peerID int) bool       // Rate Limit 방어
	//IsFinalized(hash string) bool                  // 불필요한 과거 요청 방어
}

// MessageValidator 인터페이스
type MessageValidator interface {
	Validate(msg *types.Message, ctx ValidatorContext) error
}
