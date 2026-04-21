// [slasher.go]
// 네트워크 내 Byzantine behavior를 감지하고 증거를 수집하며,
// 합의 과정에서 해당 노드를 배제하기 위한 처벌 수치를 관리하네.

package core

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"fmt"
	"sync"
	"time"
)

// Slasher: Arachnet의 기강을 잡는 엄격한 판사님일세.
type Slasher struct {
	mu           sync.RWMutex
	evidences    map[int][]types.Evidence // Validator ID -> 증거 목록
	penaltyTable map[int]int              // Validator ID -> 벌점
	processedEv  map[string]bool          // [추가] 이미 판결이 끝난 증거물(Vertex Hash) 목록
	Config       *config.Config           // 판사님의 법전일세!
}

func NewSlasher(cfg *config.Config) *Slasher {
	return &Slasher{
		evidences:    make(map[int][]types.Evidence),
		penaltyTable: make(map[int]int),
		processedEv:  make(map[string]bool),
		Config:       cfg,
	}
}

// HandleEquivocation: 이중 투표(내부 적발) 시 호출하여 즉시 처벌하고 증거를 생성하네.
func (s *Slasher) HandleEquivocation(vtx1, vtx2 *types.Vertex) *types.Evidence {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("[SLASHER] Validator %d 의 이중 투표 적발! (Round: %d)\n", vtx1.Author, vtx1.Round)

	// 벌점 부과
	s.penaltyTable[vtx1.Author] += s.Config.Security.EquivocationPenalty

	// 고발장
	evidence := types.Evidence{
		TargetID:   vtx1.Author,
		Type:       types.MsgVertex, // 이중 투표는 Vertex 관련 위반이지
		Proof1:     vtx1,
		Proof2:     vtx2,
		ReporterID: 0, // 나중에 실제 내 ID를 넣어야 하네
	}

	s.evidences[vtx1.Author] = append(s.evidences[vtx1.Author], evidence)
	return &evidence
}

// ProcessExternalEvidence: 남이 보내온 고발장을 검토하고 판결을 내리네.
func (s *Slasher) ProcessExternalEvidence(ev *types.Evidence) {
	// 1. 이미 법정에서 쫓겨난 놈이면 제외하네.
	if s.IsSlashed(ev.TargetID) {
		return
	}

	// 2. 고발장이 논리적으로 타당한지 검사하네
	if !ev.IsValid() {
		fmt.Printf("[SLASHER] 유효하지 않은 고발장 기각 (Target: %d)\n", ev.TargetID)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 3. 판결 확정 및 벌점 집행
	fmt.Printf("[SLASHER] 타 노드의 신고 접수: Validator %d 처벌 (죄목: %s)\n", ev.TargetID, ev.Type)
	s.penaltyTable[ev.TargetID] += s.Config.Security.EquivocationPenalty
	s.evidences[ev.TargetID] = append(s.evidences[ev.TargetID], *ev)
}

// IsSlashed: 이 노드가 감옥에 갔는지 확인하네.
func (s *Slasher) IsSlashed(validatorID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 벌점이 임계치를 넘으면 이 노드의 말은 아무도 안 믿게 될 걸세.
	return s.penaltyTable[validatorID] >= s.Config.Security.SlashThreshold
}

// AddDemerit: [ISlasher 인터페이스 구현]
// Orphanage 등 내부 모듈에서 가벼운 위반 사항을 보고할 때 사용하네.
func (s *Slasher) AddDemerit(author int, amount int, vtx *types.Vertex, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 일사부재리 및 처벌 임계치 체크
	if s.penaltyTable[author] >= s.Config.Security.SlashThreshold || s.processedEv[vtx.Hash] {
		return
	}

	// 2. 증거물 마킹
	s.processedEv[vtx.Hash] = true

	// 3. 고발장 생성 및 벌점 부과
	evidence := types.Evidence{
		TargetID:    author,
		Type:        types.MsgInvalidPayload,
		Proof1:      vtx,
		ReporterID:  s.Config.NodeID,
		Description: reason,
		Timestamp:   time.Now().Unix(),
	}

	s.penaltyTable[author] += amount
	s.evidences[author] = append(s.evidences[author], evidence)

	fmt.Printf("[SLASHER] Validator %d 벌점 %d 부과! (Reason: %s, Hash: %s)\n", author, amount, reason, vtx.Hash)
}
