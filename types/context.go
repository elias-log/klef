package types

// StateReader: 궁금한 걸 물어보는 채널
type StateReader interface {
	GetCurrentRound() int
	IsKnownVertex(hash string) bool
	IsRequestPending(hash string) bool
	IsDuplicateMessage(peerID int, seq uint64) bool
	UpdateAndCheckRateLimit(peerID int) bool
	IsFinalized(hash string) bool
}

// ConsensusContext: 합의를 위해 행동하는 채널
type ConsensusContext interface {
	StateReader // 읽기 기능을 포함하네
	GetID() int
	GetReadyQuorum() ([]*Message, int)
	AlreadyProposed(round int) bool
	MarkAsProposed(round int)
	Sign(data string) Signature
}
