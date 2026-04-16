package core

import (
	"arachnet-bft/types"
	"crypto/ed25519"
)

// LocalSigner: 실제 ed25519 라이브러리를 래핑한 예시일세.
type LocalSigner struct {
	privKey ed25519.PrivateKey
	pubKey  ed25519.PublicKey
}

// NewLocalSigner: 키 쌍을 생성하거나 로드하네.
func NewLocalSigner() *LocalSigner {
	pub, priv, err := ed25519.GenerateKey(nil) // nil이면 무작위 생성일세.
	if err != nil {
		panic("암호학 엔진 시동 실패!")
	}
	return &LocalSigner{
		privKey: priv,
		pubKey:  pub,
	}
}

func (s *LocalSigner) Sign(data []byte) types.Signature {
	return types.Signature(ed25519.Sign(s.privKey, data))
}

func (s *LocalSigner) GetPublicKey() types.PublicKey {
	return types.PublicKey(s.pubKey)
}

func (s *LocalSigner) Verify(data []byte, sig types.Signature, pub types.PublicKey) bool {
	return ed25519.Verify(ed25519.PublicKey(pub), data, []byte(sig))
}
