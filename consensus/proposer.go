/*
	<Objectives: 2f+1 QC 기반 새 Vertex 생성 및 배포>
	1. 부모 수집 (2f+1): 부모가 신뢰의 증거(QC).
	2. 새로운 Vertex 생성: 수집된 부모들의 해시를 Parents 목록에 넣고,
	   내 Mempool에서 transaction을 담아 Payload에 넣은 후 개인키로 서명하네.
	3. 배포 (Proposal): 완성된 Vertex를 MsgVertex 봉투에 담아 모든 Peer에게 뿌리네
	4. QC: 다른 노드들이 이 Vertex를 보고 "음, 부모들도 확실하고 서명도 맞군"이라고 판단하면 MsgVote를 돌려줄 걸세.
	그 표들이 모여 다시 다음 라운드의 부모(2f+1)가 되는 선순환 구조

	<Invariants>
	[Safety] Round Monotonicity: 동일한 Author는 동일한 Round에 오직 하나의 Vertex만 제안해야 하네. (Equivocation 방지)
	[Validity] Quorum Dependency: 새로 생성된 Vertex의 Parents 집합은 반드시 QuorumPolicy가 승인한 정족수(2f+1) 이상을 포함해야 하네.
	[Liveness] Parent Connectivity: 제안된 Vertex의 부모들은 반드시 바로 직전 라운드(R−1) 혹은 그 이전의 유효한 노드여야 하며, 끊어진 경로가 있어서는 안 되네.
	[Integrity] Signature Authenticity: 배포되는 모든 Vertex는 제안자의 유효한 개인키 서명을 포함해야 하네.
*/

package consensus

import (
	"arachnet-bft/core"
	"arachnet-bft/network"
	"arachnet-bft/types"
	"sync"
	"time"
)

type Proposer struct {
	mu            sync.Mutex
	Validator     *core.Validator
	QuorumManager QuorumPolicy
	Broadcaster   network.Broadcaster
}

// Propose: $2f+1$ 부모를 확인하고 새 Vertex를 만들어 세상에 알리네!
func (p *Proposer) Propose() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. DAG에서 부모 수집
	parents := p.Validator.DAG.GetVerticesByRound(p.Validator.Round)

	// 2. 투표 수집 및 정족수 확인: 그 부모들이 정당하다는 투표(Votes)를 가져와서 QC를 만드세.
	votes := p.Validator.DAG.GetVotesForVertices(parents)
	if len(votes) == 0 {
		return
	}

	// 3. QC, Vertex 조립
	// assembleQC 내부에서 types.Vote로 캐스팅하여 VertexHash와 Signature를 추출하네.
	qc := p.assembleQC(votes)
	if !p.QuorumManager.IsQuorum(qc) {
		return // 아직 정족수(2f+1 등)를 채우지 못했구먼.
	}

	vtx := &types.Vertex{
		Author:    p.Validator.ID,
		Round:     p.Validator.Round + 1,
		Parents:   p.extractHashesFromVertices(parents),
		ParentQC:  qc,
		Timestamp: time.Now().UnixMilli(),
		Payload:   p.fetchTransactions(),
	}

	// 서명은 Validator의 권한을 빌려 수행하세.
	vtx.Hash = vtx.CalculateHash()
	vtx.Signature = p.Validator.Sign(vtx.Hash)

	// 4. 네트워크 전파
	p.Broadcaster.Broadcast(types.MsgVertex, vtx)

	// 여기서 라운드를 올리지 마세!
	// "내가 제안한 이 Vertex가 남들에게 인정받아 다시 투표가 모일 때" 그때 올라가는 걸세.
}

// fetchTransactions: Mempool에서 가져올 로직의 자리일세.
func (p *Proposer) fetchTransactions() [][]byte {
	// TODO: Mempool 구현 시 연결
	return [][]byte{}
}

// extractSignerIDs: 투표 묶음에서 서명자 ID들만 뽑아내네.
func (p *Proposer) extractSignerIDs(votes []*types.Message) []int {
	ids := make([]int, 0, len(votes))
	for _, msg := range votes {
		ids = append(ids, msg.FromID)
	}
	return ids
}

// assembleQC: Message -> Vote -> QC
func (p *Proposer) assembleQC(votes []*types.Message) *types.QC {
	signatures := make(map[int][]byte)
	var targetHash string
	var round int

	for _, msg := range votes {
		// [핵심] Payload에서 Vote 정보를 안전하게 꺼내는 과정일세.
		if vote, ok := msg.Payload.(types.Vote); ok {
			signatures[msg.FromID] = vote.Signature
			targetHash = vote.VertexHash
			round = vote.Round
		}
	}

	return &types.QC{
		Type:       types.QCGlobal, //일단 글로벌로 가세
		VertexHash: targetHash,
		Round:      round,
		ProposerID: p.Validator.ID,
		Signatures: signatures,
	}
}

// extractHashesFromVertices: 부모 Vertex 객체들로부터 해시 목록만 뽑아내네.
func (p *Proposer) extractHashesFromVertices(vtxs []*types.Vertex) []string {
	hashes := make([]string, len(vtxs))
	for i, v := range vtxs {
		hashes[i] = v.Hash
	}
	return hashes
}
