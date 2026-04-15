package types

// MessageType을 상수로 정의하면 오타를 줄일 수 있네!
const (
	MsgVertex   = "VERTEX"
	MsgVote     = "VOTE"
	MsgFetchReq = "FETCH_REQ"
	MsgFetchRes = "FETCH_RES"
)

// 모든 통신의 기본 봉투라네.
type Message struct {
	FromID       int
	CurrentRound int
	Type         string      // MsgVertex, MsgVote 등
	Payload      interface{} // 실제 데이터
}

// FetchRequest: "나 이 해시들 좀 알려주게!"
type FetchRequest struct {
	MissingHashes []string
}

// FetchResponse: 요청받은 Vertex들을 담아 보내는 바구니일세.
type FetchResponse struct {
	Vertices []*Vertex
}
