// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package validation

import "klef/pkg/types"

/// MessageValidator defines the universal interface for all protocol message validation logic.
type MessageValidator interface {
	// Validate checks the consistency and authenticity of a message against the current system state.
	Validate(msg *types.Message, ctx types.ConsensusContext) error
}
