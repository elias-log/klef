// 누락된 Vertex를 이웃 노드에게 요청하여 DAG의 구멍을 메움.
// TODO: [Fetcher Evolution & Integration]
//
// 1. 이벤트 기반 단계 전환 (Liveness): [Phase 1]
//    - 현재 Step 1~3은 time.Sleep 기반의 정적 타이밍에 의존함.
//    - 의도: 데이터가 도착하는 즉시 다음 단계를 취소하거나 진행하는
//      Event-driven 방식으로 전환하여 불필요한 지연(Latency)을 제거해야 함.
//
// 2. Pending 시스템과의 완벽한 결합 (Control): [Phase 1]
//    - 현재 Fetcher가 스스로 재시도를 제어하고 있음.
//    - 의도: 향후 Validator의 'startPendingCleanup' 루프가 재시도 타이밍을 결정하고,
//      Fetcher는 단순 '요청 발송기' 역할만 수행하도록 로직을 이관해야 함.
//
// 3. Peer 평판 시스템 연계 (Safety): [Phase 2]
//    - handlePanic 시점에 단순히 출력만 하는 것이 아니라,
//      데이터를 주지 않는 노드(Suspect)에 대한 패널티 부여 로직을 연계해야 함.

// TODO: [Critical Safety Fixes]
// 1. [Phase 0] InboundResponse 채널을 Hash 기반의 분기 처리(Event-Multiplexing)로 개선할 것. (Global Wake-up 방지)
// 2. [Phase 2] SuspectID가 응답하지 않을 경우 Step 2로 즉시 전이하는 Adaptive Timeout 고려.

// Done: timer.Reset 호출 전 반드시 채널 비우기(Drain) 로직을 추가하여 레이스 컨디션 차단.

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

// StartSync: 특정 해시의 Vertex가 없을 때 네트워크에 요청을 보냄
func (f *VertexFetcher) StartSync(missingHashes []string, suspectID int) {

	// [PendingManager 적용] Hash 단위 필터링
	filtered := make([]string, 0)
	for _, h := range missingHashes {
		if !f.Validator.pendingMgr.IsPending(h) {
			f.Validator.pendingMgr.Add(h)
			filtered = append(filtered, h)
		}
	}

	// 보낼 게 없으면 즉시 퇴근!
	if len(filtered) == 0 {
		return
	}

	go func() {
		currentMissing := filtered
		timer := time.NewTimer(time.Hour)
		if !timer.Stop() {
			<-timer.C
		}
		defer timer.Stop()

		// [단계별 시나리오: Step 1 ~ 3]
		// Step 1: Direct Check (Suspect에게 먼저 확인)
		// Step 2: Neighbor Fan-out
		// Step 3: Network-wide Broadcast
		for step := 1; step <= 3; step++ {
			// 1. 현재 누락된 게 있는지 최종 확인
			currentMissing = f.Validator.DAG.GetMissingHashes(currentMissing)
			if len(currentMissing) == 0 {
				return // 다 찾았으니 일찍 퇴근하세!
			}

			// 2. 단계에 맞는 대상에게 요청 발송
			f.dispatchByStep(step, suspectID, currentMissing)
			// [Safety Fix] Timer Reset 전 확실하게 채널을 비워두어야 하네
			if !timer.Stop() {
				select {
				case <-timer.C: // 이미 만료되었다면 채널에서 비워주기
				default: // 이미 비어있다면 대기 없이 통과
				}
			}
			timer.Reset(f.getStepTimeout(step))

		waitLoop:
			for {
				select {
				case <-f.InboundResponse:
					// 응답이 하나라도 오면, 다시 누락분을 체크하세.
					currentMissing = f.Validator.DAG.GetMissingHashes(currentMissing)
					if len(currentMissing) == 0 {
						return // 다 왔구먼! 고루틴 종료.
					}
					// 아직 다 안 왔으면 계속 기다리거나 다음 단계로 넘어갈 준비를 하네.

				case <-timer.C:
					// 타임아웃! 다음 단계로 넘어가서 더 넓게 물어봐야 하네.
					break waitLoop

				case <-f.Validator.ctx.Done():
					// 시스템 종료 시 고루틴도 종료하네.
					return
				}
			}
		}

		// Step 4: 패닉 처리
		// 2f+1이 찬성한 데이터가 이때까지 안 오면 문제일세.
		finalMissing := f.Validator.DAG.GetMissingHashes(currentMissing)
		if len(finalMissing) > 0 {
			f.handlePanic(finalMissing)
		}
	}()
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
		FetchReq: &types.FetchRequest{
			MissingHashes: hashes,
		},
	}
}

func (f *VertexFetcher) dispatchByStep(step int, suspectID int, hashes []string) {
	switch step {
	case 1:
		f.dispatchFetch([]int{suspectID}, hashes)
	case 2:
		neighbors := f.getFilteredNeighbors(f.Validator.Config.Sync.MaxRandomPeers, suspectID)
		f.dispatchFetch(neighbors, hashes)
	case 3:
		f.Validator.Broadcast(types.MsgFetchReq, &types.FetchRequest{
			MissingHashes: hashes,
		})
	}
}

func (f *VertexFetcher) getStepTimeout(step int) time.Duration {
	switch step {
	case 1:
		return f.Validator.Config.Sync.Step1Timeout
	case 2:
		return f.Validator.Config.Sync.Step2Timeout
	case 3:
		return f.Validator.Config.Sync.Step3Timeout
	default:
		return 5 * time.Second
	}
}

// dispatchFetch: 지정된 피어들에게 실제 FETCH_REQ 메시지를 전송하는 '발송' 담당일세.
func (f *VertexFetcher) dispatchFetch(peerIDs []int, hashes []string) {
	if len(peerIDs) == 0 || len(hashes) == 0 {
		return
	}

	payload := &types.FetchRequest{
		MissingHashes: hashes,
	}
	for _, pid := range peerIDs {
		f.Validator.SendTo(pid, types.MsgFetchReq, payload)
	}
}

func (f *VertexFetcher) handlePanic(hashes []string) {
	// 로그를 남기고, 이 경로의 Vertex 처리를 중단하거나
	// 상위 계층에 '데이터 유실' 경보를 울려야 하네.
	// 이건 단순한 네트워크 지연이 아니라 데이터 Availability 문제일세.
	// 검증 중단: 해당 Vertex를 OrphanBuffer에서 영구히 지우거나, 일정 시간 뒤에 아주 긴 주기로 다시 시도하는 'Cold Storage'로 보내야 하네.
	// Slashing: 만약 특정 노드가 계속 데이터를 안 준다면, 그놈을 Byzantine으로 간주하고 평판을 깎아야 하네.
	fmt.Printf("[CRITICAL] Data missing after full broadcast: %v\n", hashes)
	// TODO: 여기서 지수 백오프가 최대치(15회)에 도달했음에도 못 찾았다면
	// 이 해시들을 'Blacklist'에 넣거나 관리자에게 알람을 주는 로직을 고민해봄직하네.
}
