package types

type MessageType uint8

const (
	MsgVertex MessageType = iota
	MsgVote
	MsgFetchReq
	MsgFetchRes
	MsgEvidence
)

func (t MessageType) String() string {
	switch t {
	case MsgVertex:
		return "VERTEX"
	case MsgVote:
		return "VOTE"
	case MsgFetchReq:
		return "FETCH_REQ"
	case MsgFetchRes:
		return "FETCH_RES"
	case MsgEvidence:
		return "EVIDENCE"
	default:
		return "UNKNOWN"
	}
}

type Message struct {
	FromID       int
	CurrentRound int
	Type         MessageType // MsgVertex, MsgVote 등
	Signature    []byte

	// Payload
	Vote     *Vote
	Vertex   *Vertex
	FetchReq *FetchRequest
	FetchRes *FetchResponse
	Evidence *Evidence
}

// FetchRequest: "나 이 해시들 좀 알려주게!"
type FetchRequest struct {
	MissingHashes []string
}

// FetchResponse: 요청받은 Vertex들을 담아 보내는 바구니일세.
type FetchResponse struct {
	Vertices []*Vertex
}

/*
  [Evidence Handling Rules]
  1. 무결성: 모든 Proof는 해당 Author의 유효한 서명을 포함해야 하네.
  2. 가벼움: 나중에는 Vertex 전체 대신, (해시 + 서명)만 담아 패킷 크기를 줄이세.
  3. 전파: 한 번 검증된 증거는 모든 이웃 노드에게 'Gossip' 프로토콜로 빛의 속도로 퍼뜨리게나.
*/
// 범행 증거
type Evidence struct {
	TargetID int         // 고발당한 노드의 ID
	Type     MessageType // 죄목 (예: MsgVertex - 이중 투표 등)
	Proof1   *Vertex     // 첫 번째 물증 (예: 첫 번째로 생성한 Vertex)
	Proof2   *Vertex     // 두 번째 물증 (예: 같은 라운드에 생성한 또 다른 Vertex)

	// 이 증거를 처음 발견하고 서명한 신고자 정보
	ReporterID int
	Timestamp  int64 // 신고 시각 (UNIX)
}

// IsValid: 증거가 논리적으로 타당한지 스스로 검사하네.
func (e *Evidence) IsValid() bool {
	// 1. 범인이 동일인물인가?
	if e.Proof1.Author != e.Proof2.Author || e.Proof1.Author != e.TargetID {
		return false
	}

	// 2. 이중 투표(Equivocation)인 경우: 라운드는 같은데 해시가 다른가?
	if e.Proof1.Round == e.Proof2.Round && e.Proof1.Hash != e.Proof2.Hash {
		return true
	}

	// 다른 죄목(라운드 점프 등) 검증 로직을 추가할 예정이네.
	return false
}
