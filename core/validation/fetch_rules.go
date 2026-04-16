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

func (f *FetchRequestValidator) Validate(msg *types.Message, ctx ValidatorContext) error {
	req, ok := msg.Payload.(types.FetchRequest)
	if !ok {
		return errors.New("payload is not FetchResponse")
	}

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

func (f *FetchResponseValidator) Validate(msg *types.Message, ctx ValidatorContext) error {
	res, ok := msg.Payload.(types.FetchResponse)
	if !ok {
		return errors.New("payload is not FetchResponse")
	}

	// 1. 응답 데이터 개수 검증 (Dos 방어)
	if len(res.Vertices) == 0 {
		return errors.New("empty fetch response")
	}
	if f.MaxVertexCount > 0 && len(res.Vertices) > f.MaxVertexCount {
		return errors.New("too many vertices in response")
	}

	currRound := ctx.GetCurrentRound()

	// 2. 각 Vertex의 개별 무결성 및 의미 검증
	for _, vtx := range res.Vertices {
		// [검증] 결정론적 해시 무결성
		if vtx.Hash != vtx.CalculateHash() {
			return fmt.Errorf("hash mismatch for vertex: %s", vtx.Hash)
		}

		// [검증] 요청한 vertex가 맞는지 확인
		if !ctx.IsRequestPending(vtx.Hash) {
			return errors.New("unsolicited vertex received: i didn't ask for this!")
		}

		// [검증] 라운드 범위 (미래 & 과거 방어)
		// 너무 먼 미래(+10) 데이터는 프로토콜 위반일세.
		if vtx.Round > currRound+10 {
			return fmt.Errorf("vertex %s is too far in future (Round: %d, Current: %d)", vtx.Hash, vtx.Round, currRound)
		}
		// 너무 오래된 데이터(예: 50라운드 전)는 메모리 낭비이니 거절하세.
		if currRound > 50 && vtx.Round < currRound-50 {
			return fmt.Errorf("vertex %s is too old (Round: %d, Current: %d)", vtx.Hash, vtx.Round, currRound)
		}

		// [검증 C] 중복 검증 (이미 우리 DAG에 있는가?)
		if ctx.IsKnownVertex(vtx.Hash) {
			// 이건 에러라기보다 효율성의 문제니, 로그만 남기고 넘어가도 좋네.
			continue
		}

		// [검증 D] 구조적 정합성 기초 확인
		// 0라운드(Genesis)가 아닌데 부모가 없다면 가짜일세.
		if vtx.Round > 0 && len(vtx.Parents) == 0 {
			return fmt.Errorf("non-genesis vertex %s has no parents", vtx.Hash)
		}

		// 자기 자신을 부모로 가질 수는 없네 (DAG의 기본 원칙!)
		for _, pHash := range vtx.Parents {
			if pHash == vtx.Hash {
				return fmt.Errorf("vertex %s contains itself as a parent (Circular!)", vtx.Hash)
			}
		}
	}

	return nil
}
