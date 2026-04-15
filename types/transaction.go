package types

type Transaction struct {
	ID      uint64
	Payload []byte // 실제 데이터 (예: "A가 B에게 10코인 전송")
}
