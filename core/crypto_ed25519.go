package core

import (
	"arachnet-bft/types"
	"crypto/ed25519"
	"fmt"
)

type Ed25519Signer struct {
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

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

func (s *Ed25519Signer) Sign(data []byte) types.Signature {
	return types.Signature(ed25519.Sign(s.privKey, data))
}

func (s *Ed25519Signer) GetPublicKey() types.PublicKey {
	return types.PublicKey(s.pubKey)
}

func (s *Ed25519Signer) Verify(data []byte, sig types.Signature, pub types.PublicKey) bool {
	// ed25519.Verify는 인자로 []byte를 받으니 형변환이 필요하네.
	return ed25519.Verify(ed25519.PublicKey(pub), data, []byte(sig))
}
