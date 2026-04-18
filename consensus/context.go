package consensus

import "arachnet-bft/types"

// Proposer가 Validator에게 원하는 최소한의 기능
type ProposerContext interface {
	GetID() int
	IsKnownVertex(hash string) bool
	GetCurrentRound() int
	GetReadyQuorum() ([]*types.Message, int)
	AlreadyProposed(round int) bool
	MarkAsProposed(round int)
	Sign(data string) types.Signature
}
