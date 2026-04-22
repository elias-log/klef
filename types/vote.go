// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package types

type Vote struct {
	Round      int
	VertexHash string
	VoterID    int
	Signature  []byte
}
