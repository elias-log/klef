package types

// PublicKey는 노드의 공개키를 나타내네.
type PublicKey []byte

// Signature 역시 바이트 덩어리일세.
type Signature []byte

// Signer: 서명 기능을 추상화한 인터페이스일세.
// 나중에 Ed25519Signer, BLSSigner 등으로 구현체를 갈아끼울 생각이네.
type Signer interface {
	Sign(data []byte) Signature
	GetPublicKey() PublicKey
	Verify(data []byte, sig Signature, pub PublicKey) bool
}
