package types

type Vote struct {
	Round      int
	VertexHash string
	VoterID    int
	Signature  []byte
}
