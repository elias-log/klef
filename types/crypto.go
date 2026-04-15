package types

// PublicKey는 노드의 공개키를 나타내네.
// 나중에 ed25519나 ecdsa 패키지의 타입을 쓰게 되겠지만, 일단 정의해두세!
type PublicKey []byte

// Signature 역시 바이트 덩어리일세.
type Signature []byte
