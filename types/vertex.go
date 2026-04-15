// Vertex 및 QC 구조 정의

package types

// Vertex는 DAG의 기본 단위라네.
// 대문자 V로 시작해야 밖에서 보이네!
type Vertex struct {
	Author    int      // 생성자 ID
	Round     int      // 라운드 번호
	Parents   []string // 부모 Vertex들의 해시 리스트
	ParentQC  *QC      // 부모들에 대한 Quorum Certificate
	Payload   [][]byte // 포함된 트랜잭션들
	Signature []byte   // 생성자의 서명
	Hash      string   // 이 Vertex의 고유 식별자 (보통 SHA-256)
}

// QC는 특정 라운드의 Vertex에 대해 2f+1개의 서명이 모였다는 증명일세.
type QC struct {
	VertexHash string
	Round      int
	Signatures [][]byte
}
