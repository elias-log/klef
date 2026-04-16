//누락된 Vertex를 이웃 노드에게 요청하여 DAG의 구멍을 메움.

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

	go func() {
		// Step 1: 유포자(Suspect)에게 먼저 확인 (Direct Check)
		f.dispatchFetch([]int{suspectID}, missingHashes)
		time.Sleep(f.Validator.Config.SyncStep1Timeout)

		// Step 2: Neighbor Fan-out
		// GetMissingHashes로 필터링해서, 이미 도착한 건 제외하세!
		stillMissing := f.Validator.DAG.GetMissingHashes(missingHashes)
		if len(stillMissing) > 0 {
			// 이미 물어본 suspectID는 제외하고 무작위 피어 선정
			neighbors := f.getFilteredNeighbors(f.Validator.Config.MaxRandomPeers, suspectID)
			f.dispatchFetch(neighbors, stillMissing)
			time.Sleep(f.Validator.Config.SyncStep2Timeout)
		} else {
			return // 다 왔으면 일찍 퇴근하세!
		}

		// Step 3: Network-wide Broadcast
		stillMissing = f.Validator.DAG.GetMissingHashes(stillMissing)
		if len(stillMissing) > 0 {
			f.Validator.Broadcast(f.makeFetchReq(stillMissing))
			time.Sleep(f.Validator.Config.SyncStep3Timeout)
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
