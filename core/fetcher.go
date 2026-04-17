// 누락된 Vertex를 이웃 노드에게 요청하여 DAG의 구멍을 메움.
// TODO: [Fetcher Evolution & Integration]
//
// 1. 이벤트 기반 단계 전환 (Liveness):
//    - 현재 Step 1~3은 time.Sleep 기반의 정적 타이밍에 의존함.
//    - 의도: 데이터가 도착하는 즉시 다음 단계를 취소하거나 진행하는
//      Event-driven 방식으로 전환하여 불필요한 지연(Latency)을 제거해야 함.
//
// 2. Pending 시스템과의 완벽한 결합 (Control):
//    - 현재 Fetcher가 스스로 재시도를 제어하고 있음.
//    - 의도: 향후 Validator의 'startPendingCleanup' 루프가 재시도 타이밍을 결정하고,
//      Fetcher는 단순 '요청 발송기' 역할만 수행하도록 로직을 이관해야 함.
//
// 3. Batch 요청의 세분화 관리 (Efficiency):
//    - 현재 요청은 []hash(Batch) 단위이나, Pending 관리는 개별 Hash 단위임.
//    - 의도: 일부 데이터만 누락된 경우, 전체 배치를 다시 요청하지 않고
//      정말 없는 녀석만 골라내는 'stillMissing' 필터링을 매 단계마다 더 정교하게 수행할 것.
//
// 4. Peer 평판 시스템 연계 (Safety):
//    - handlePanic 시점에 단순히 출력만 하는 것이 아니라,
//      데이터를 주지 않는 노드(Suspect)에 대한 패널티 부여 로직을 연계해야 함.

package core

import (
	"arachnet-bft/types"
	"fmt"
	"time"
)

type VertexFetcher struct {
	InboundResponse chan *types.Vertex // 요청한 데이터를 받을 채널
	Validator       *Validator         // 부모 노드 정보와 통신 인터페이스를 쓰기 위함일세
}

// getFilteredNeighbors: 물어볼 이웃을 고르되, 이미 물어본 suspectID는 제외하네.
func (f *VertexFetcher) getFilteredNeighbors(count int, suspectID int) []int {
	// Validator를 통해 주변 피어들을 가져오네.
	peers := f.Validator.GetRandomPeers(count)

	filtered := make([]int, 0, len(peers))
	for _, pid := range peers {
		if pid != suspectID {
			filtered = append(filtered, pid)
		}
	}
	return filtered
}

// makeFetchReq: 누락된 해시들을 담은 FETCH_REQ 타입의 공통 메시지를 생성하네.
func (f *VertexFetcher) makeFetchReq(hashes []string) *types.Message {
	return &types.Message{
		FromID: f.Validator.ID,
		Type:   types.MsgFetchReq, // "FETCH_REQ" 상수를 사용하세.
		Payload: types.FetchRequest{
			MissingHashes: hashes,
		},
	}
}

// StartSync: 특정 해시의 Vertex가 없을 때 네트워크에 요청을 보냄
func (f *VertexFetcher) StartSync(missingHashes []string, suspectID int) {
	// 1. 요청 관리 상태 (이건 구조체 멤버로 두는 게 좋네)
	// type RequestState struct { attempts int, askedPeers map[int]bool }

	// Hash 단위 필터링으로 고루틴 폭발 방어
	filtered := make([]string, 0)
	for _, h := range missingHashes {
		// 이미 요청 중인 해시는 중복 요청하지 않네!
		if !f.Validator.IsRequestPending(h) {
			f.Validator.AddPendingRequest(h)
			filtered = append(filtered, h)
		}
	}

	// 보낼 게 없으면 즉시 퇴근!
	if len(filtered) == 0 {
		return
	}

	go func() {
		// Step 1: 유포자(Suspect)에게 먼저 확인 (Direct Check)
		f.dispatchFetch([]int{suspectID}, filtered)
		time.Sleep(f.Validator.Config.Sync.Step1Timeout)

		// Step 2: Neighbor Fan-out
		// GetMissingHashes로 필터링해서, 이미 도착한 건 제외하세!
		stillMissing := f.Validator.DAG.GetMissingHashes(filtered)
		if len(stillMissing) == 0 {
			return
		}
		// 이미 물어본 suspectID는 제외하고 무작위 피어 선정
		neighbors := f.getFilteredNeighbors(f.Validator.Config.Sync.MaxRandomPeers, suspectID)
		f.dispatchFetch(neighbors, stillMissing)
		time.Sleep(f.Validator.Config.Sync.Step2Timeout)

		// Step 3: Network-wide Broadcast
		stillMissing = f.Validator.DAG.GetMissingHashes(stillMissing)
		if len(stillMissing) > 0 {
			f.Validator.Broadcast(f.makeFetchReq(stillMissing))
			time.Sleep(f.Validator.Config.Sync.Step3Timeout)
		} else {
			return
		}

		// Step 4: 그래도 없으면 handlepanic (비상사태)
		// 2f+1이 찬성한 데이터가 이때까지 안 오면 문제일세.
		stillMissing = f.Validator.DAG.GetMissingHashes(stillMissing)
		if len(stillMissing) > 0 {
			f.handlePanic(stillMissing)
		}
	}()
}

// dispatchFetch: 지정된 피어들에게 실제 FETCH_REQ 메시지를 전송하는 '발송' 담당일세.
func (f *VertexFetcher) dispatchFetch(peerIDs []int, hashes []string) {
	if len(peerIDs) == 0 || len(hashes) == 0 {
		return
	}

	req := f.makeFetchReq(hashes)
	for _, pid := range peerIDs {
		f.Validator.SendTo(pid, req)
	}
}

func (f *VertexFetcher) handlePanic(hashes []string) {
	// 로그를 남기고, 이 경로의 Vertex 처리를 중단하거나
	// 상위 계층에 '데이터 유실' 경보를 울려야 하네.
	// 이건 단순한 네트워크 지연이 아니라 데이터 Availability 문제일세.
	// 검증 중단: 해당 Vertex를 OrphanBuffer에서 영구히 지우거나, 일정 시간 뒤에 아주 긴 주기로 다시 시도하는 'Cold Storage'로 보내야 하네.
	// Slashing: 만약 특정 노드가 계속 데이터를 안 준다면, 그놈을 Byzantine으로 간주하고 평판을 깎아야 하네.
	fmt.Printf("[CRITICAL] Data missing after full broadcast: %v\n", hashes)
}
