// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package types

type PublicKey []byte
type Signature []byte
type Signer interface {
	Sign(data []byte) Signature
	GetPublicKey() PublicKey
	Verify(data []byte, sig Signature, pub PublicKey) bool
}
