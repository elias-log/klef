/*
   [TODO: Unsolicited Response Defense (Request-Response Matching)]

   현재는 받은 Vertex의 '내용물'만 검증하고 있네. (Stateless Validation)
   악의적인 피어가 요청하지 않은 데이터를 폭탄처럼 던지는 'Spamming 공격'을 막으려면:

   1. Validator/Fetcher에 map[Hash]time.Time 형태의 PendingRequests 장부를 둘 것.
   2. FetchRequest를 보낼 때 장부에 기록하고, Response를 받으면 장부에 있는지 확인(WasRequested).
   3. 장부에 없는 데이터는 검증 비용(CPU)을 쓰기 전에 즉각 폐기할 것.
*/

package validation

import (
	"arachnet-bft/types"
	"errors"
	"fmt"
)

// FetchRequestValidator: 상대방이 보낸 FETCH_REQ 메시지를 검증하네.
type FetchRequestValidator struct {
	MaxRequestHashes int // 한 번에 요청할 수 있는 최대 해시 수
}

func (f *FetchRequestValidator) Validate(msg *types.Message, ctx types.StateReader) error {

	if msg.FetchReq == nil {
		return errors.New("message does not contain a FetchRequest")
	}
	req := msg.FetchReq

	// 1. 최소 1개 이상의 해시는 요청해야 하네.
	if len(req.MissingHashes) == 0 {
		return errors.New("empty missing hashes in request")
	}

	// 2. 너무 많은 해시를 요청하면 Dos 공격으로 간주하네.
	if f.MaxRequestHashes > 0 && len(req.MissingHashes) > f.MaxRequestHashes {
		return fmt.Errorf("too many hashes requested: %d", len(req.MissingHashes))
	}

	return nil
}

// FetchResponseValidator: 상대방이 보낸 FETCH_RES 메시지를 검증하네.
type FetchResponseValidator struct {
	MaxVertexCount int
}

func (f *FetchResponseValidator) Validate(msg *types.Message, ctx types.StateReader) error {
	if msg.FetchRes == nil {
		return errors.New("message does not contain a FetchResponse")
	}
	res := msg.FetchRes

	// 1. 응답 데이터 개수 검증 (Dos 방어)
	if len(res.Vertices) == 0 {
		return errors.New("empty fetch response")
	}
	if f.MaxVertexCount > 0 && len(res.Vertices) > f.MaxVertexCount {
		return errors.New("too many vertices in response")
	}

	// 2. 각 Vertex의 개별 무결성 및 의미 검증
	for _, vtx := range res.Vertices {
		// 1. [비용 최저] 이미 아는 데이터인가? (중복 방어)
		if ctx.IsKnownVertex(vtx.Hash) {
			continue
		}

		// 2. [비용 저] 내가 요청한 데이터인가? (Spam 방어)
		// 이 검증이 해시 계산보다 앞에 오는 것이 CPU 보호에 유리하네.
		if !ctx.IsRequestPending(vtx.Hash) {
			return errors.New("unsolicited vertex received: spam attack suspected")
		}

		// 3. [비용 중] 라운드 범위 체크 (가장 기본적인 프로토콜 위반 확인)
		currRound := ctx.GetCurrentRound()
		if vtx.Round > currRound+10 || (currRound > 50 && vtx.Round < currRound-50) {
			return fmt.Errorf("vertex %s round out of valid range", vtx.Hash)
		}

		// 4. [비용 고] 해시 무결성 검증 (이제야 비로소 CPU를 써서 계산하세!)
		if vtx.Hash != vtx.CalculateHash() {
			return fmt.Errorf("hash mismatch for vertex: %s", vtx.Hash)
		}

		// 5. [구조 검증] DAG 규칙 확인
		// 0라운드(Genesis)가 아닌데 부모가 없다면 가짜일세.
		if vtx.Round > 0 && len(vtx.Parents) == 0 {
			return fmt.Errorf("non-genesis vertex %s has no parents", vtx.Hash)
		}

		parentSet := make(map[string]struct{})
		for _, pHash := range vtx.Parents {
			if pHash == vtx.Hash {
				return fmt.Errorf("circular reference in vertex %s", vtx.Hash)
			}
			if _, exists := parentSet[pHash]; exists {
				return fmt.Errorf("duplicate parent %s in vertex %s", pHash, vtx.Hash)
			}
			parentSet[pHash] = struct{}{}
		}
		return nil
	}

	return nil
}
