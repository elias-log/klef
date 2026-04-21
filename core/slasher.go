package core

import (
	"arachnet-bft/config"
	"arachnet-bft/types"
	"fmt"
	"sync"
	"time"
)

type Slasher struct {
	mu           sync.RWMutex
	evidences    map[int][]types.Evidence // Validator ID -> 증거 목록
	penaltyTable map[int]int              // Validator ID -> 벌점
	processedEv  map[string]bool          // 이미 판결이 끝난 증거물(Vertex Hash) 목록
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

// HandleEquivocation: 이중 투표시 호출하여 즉시 처벌하고 증거를 생성하네.
func (s *Slasher) HandleEquivocation(vtx1, vtx2 *types.Vertex) *types.Evidence {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Printf("[SLASHER] Validator %d 의 이중 투표 적발! (Round: %d)\n", vtx1.Author, vtx1.Round)

	// 벌점 부과
	s.penaltyTable[vtx1.Author] += s.Config.Security.EquivocationPenalty

	// 고발장
	evidence := types.Evidence{
		TargetID:   vtx1.Author,
		Type:       types.MsgVertex,
		Proof1:     vtx1,
		Proof2:     vtx2,
		ReporterID: 0, // TODO: 나중에 실제 내 ID를 넣어야 하네
	}

	s.evidences[vtx1.Author] = append(s.evidences[vtx1.Author], evidence)
	return &evidence
}

// ProcessExternalEvidence: 남이 보내온 고발장을 검토하고 판결을 내리네.
func (s *Slasher) ProcessExternalEvidence(ev *types.Evidence) {
	// 1. 고발장이 논리적으로 타당한지 검사하네
	if !ev.IsValid() {
		fmt.Printf("[SLASHER] 유효하지 않은 고발장 기각 (Target: %d)\n", ev.TargetID)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 2. 이미 처벌받았거나 처리된 증거(Proof1 기준)면 기각
	// Proof1.Hash가 고유한 증거 ID 역할을 한다고 가정하세.
	if s.penaltyTable[ev.TargetID] >= s.Config.Security.SlashThreshold || s.processedEv[ev.Proof1.Hash] {
		return
	}

	s.executeSlasher(
		ev.TargetID,
		s.Config.Security.EquivocationPenalty,
		ev.Proof1.Hash,
		ev.Type,
		ev.Proof1,
		ev.Proof2,
		ev.Description,
	)
}

// executeSlasher: [Internal] 실제 장부 기록을 전담하는 서기일세.
func (s *Slasher) executeSlasher(
	target int,
	amount int,
	evHash string,
	evType types.MessageType,
	p1, p2 *types.Vertex,
	reason string,
) {
	s.penaltyTable[target] += amount
	s.processedEv[evHash] = true

	evidence := types.Evidence{
		TargetID:    target,
		Type:        evType,
		Proof1:      p1,
		Proof2:      p2,
		ReporterID:  s.Config.NodeID,
		Description: reason,
		Timestamp:   time.Now().Unix(),
	}
	s.evidences[target] = append(s.evidences[target], evidence)

	fmt.Printf("[SLASHER] Validator %d 벌점 %d 부과! (Total: %d/%d, Reason: %s)\n",
		target, amount, s.penaltyTable[target], s.Config.Security.SlashThreshold, reason)
}

// GetPenalty: 테스트에서 필요한 기능일세.
func (s *Slasher) GetPenalty(validatorID int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.penaltyTable[validatorID]
}

// IsSlashed: 이 노드가 감옥에 갔는지 확인하네.
func (s *Slasher) IsSlashed(validatorID int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 벌점이 임계치를 넘으면 이 노드의 말은 아무도 안 믿게 될 걸세.
	return s.penaltyTable[validatorID] >= s.Config.Security.SlashThreshold
}

// AddDemerit: 내부 모듈 보고용
// Orphanage 등 내부 모듈에서 가벼운 위반 사항을 보고할 때 사용하네.
func (s *Slasher) AddDemerit(author int, amount int, vtx *types.Vertex, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 이미 파면된 노드거나 이미 처리된 증거물이면 기각
	if s.penaltyTable[author] >= s.Config.Security.SlashThreshold || s.processedEv[vtx.Hash] {
		return
	}

	s.executeSlasher(author, amount, vtx.Hash, types.MsgInvalidPayload, vtx, nil, reason)
}
