// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Ed25519Signer provides a thin wrapper around the standard Ed25519
implementation for signing and verification.

Key properties:
- Minimal Abstraction: Delegates all cryptographic guarantees to the
  underlying standard library.
- Deterministic Signatures: Relies on Ed25519's deterministic behavior.
- Interface Compatibility: Adapts the crypto layer to the project's
  types.Signer interface.

Note:
- This module does not implement custom cryptography.
- Security assumptions fully depend on the correctness of the Ed25519 standard library.
*/

package core

import (
	"crypto/ed25519"
	"fmt"
	"klef/pkg/types"
)

/// Ed25519Signer encapsulates private/public key pairs for cryptographic operations.
type Ed25519Signer struct {
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

/// NewEd25519Signer generates a new random key pair.
func NewEd25519Signer() (*Ed25519Signer, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("ed25519 key generation failed: %w", err)
	}
	return &Ed25519Signer{
		privKey: priv,
		pubKey:  pub,
	}, nil
}

/// Sign creates a digital signature for the given data using the private key.
func (s *Ed25519Signer) Sign(data []byte) types.Signature {
	return types.Signature(ed25519.Sign(s.privKey, data))
}

/// GetPublicKey returns the public key associated with this signer.
func (s *Ed25519Signer) GetPublicKey() types.PublicKey {
	return types.PublicKey(s.pubKey)
}

/// Verify checks if a signature is valid for the given data and public key.
func (s *Ed25519Signer) Verify(data []byte, sig types.Signature, pub types.PublicKey) bool {
	return ed25519.Verify(ed25519.PublicKey(pub), data, []byte(sig))
}
