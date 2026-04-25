// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package types

/// PublicKey represents a cryptographic public key (Ed25519 or BLS).
type PublicKey []byte

/// Signature represents a cryptographic signature.
// Note: We use []byte instead of Hash because signatures (e.g., BLS 96-byte)
// are typically larger than a 32-byte hash.
type Signature []byte

/// Signer defines the essential cryptographic operations for the Klef DAG.
type Signer interface {
	// Sign generates a signature for the given data (usually a Hash.Bytes()).
	Sign(data []byte) Signature

	// GetPublicKey returns the signer's public key.
	GetPublicKey() PublicKey

	// Verify checks if a signature is valid for the given data and public key.
	// It's defined as a method of Signer, but in some designs, this can be a static helper.
	Verify(data []byte, sig Signature, pub PublicKey) bool
}
