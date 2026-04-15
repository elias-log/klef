//메모리 속 객체를 '택배 박스(Byte)'로 포장하거나 뜯는 역할

package network

/*
   [TODO: Performance & Reliability Refinement]

   1. Chunking (청크 분할):
      - 현재 FetchResponse는 모든 Vertex를 하나의 메시지에 담고 있음.
      - 대규모 DAG에서 수백 개의 부모를 전송할 경우 패킷 크기 초과(MTU issue) 위험.
      - MaxPayloadSize(예: 1MB)를 설정하여 초과 시 Response1, Response2로 나누어 전송할 것.

   2. Prioritization (우선순위):
      - Fetch 요청이 폭주할 경우 최신 라운드(CurrentRound - 1)의 부모부터 우선 전송.
      - 오래된 라운드 데이터는 전송 우선순위를 낮추어 대역폭 최적화 필요.
*/

import (
	"bytes"
	"encoding/gob"
)

// Encode: 객체를 바이트 덩어리로! (Serialization)
func Encode(msg interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(msg); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode: 바이트를 다시 객체로!
func Decode(data []byte, target interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(target)
}
