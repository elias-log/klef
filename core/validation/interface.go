package validation

import "arachnet-bft/types"

// MessageValidator 인터페이스
type MessageValidator interface {
	Validate(msg *types.Message, ctx types.StateReader) error
}
