/*
   <Objectives: Quorum-Driven Vertex Lifecycle>
   1. 투표 기반 부모 확정 (QC Selection):
      단순히 라운드로 긁어오는 것이 아니라, Validator의 VotePool에서 2f+1 이상의
      투표가 모인 '검증된 정점들'을 추출하여 이번 라운드의 부모로 삼네.
   2. 증거(QC) 조립:
      수집된 투표 서명들을 모아 'ParentQC'를 생성하네. 이 QC는 "내 부모들은 이미
      네트워크 정족수의 승인을 받았다"는 움직일 수 없는 보증서가 될 걸세.
   3. 인과 관계가 담긴 Vertex 생성:
      QC가 가리키는 해시들을 Parents 목록에 넣고, 내 Mempool의 트랜잭션을 담아
      개인키로 서명하네.
   4. 배포 및 선순환:
      완성된 Vertex를 모든 Peer에게 전파(Broadcast)하네. 이 Vertex가 다시 남들에게
      인정받아 투표를 얻으면, 그것이 다음 라운드의 부모(QC)가 되는 구조일세.

   <Invariants>
   [Safety] Double-Proposal Guard:
       동일 Author는 동일 Round에 오직 하나의 Vertex만 제안해야 하네.
       이미 제안했다면 즉시 중단하여 이중 투표(Equivocation) 슬래싱을 방지하네.
   [Validity] Evidence-First:
       새로운 Vertex는 반드시 유효한 ParentQC를 포함해야 하며,
       이 QC는 QuorumPolicy(2f+1 등)를 완벽히 충족해야 하네.
   [Liveness] Causal Link:
       모든 부모는 이전 라운드의 유효한 노드여야 하며, QC를 통해 끊어지지 않는
       신뢰의 사슬을 증명해야 하네.
   [Integrity] Absolute Authenticity:
       배포되는 모든 Vertex는 제안자의 Ed25519 서명을 포함해야 하며,
       CalculateHash는 모든 필드를 결정론적으로 반영해야 하네.
*/

/*
   [TODO: Phase 2+ Strategic Evolution]

   1. Multi-Hash Consensus (DAG Integrity):
      - 현재는 쿼럼 투표 중 하나의 대표 해시(Representative Hash)만 QC에 담고 있네.
      - 차후 2f+1개의 투표가 각기 다른 부모를 가리킬 경우, 모든 부모 해시의 집합(Set)이나
        머클 루트를 QC에 포함하여 DAG의 인과적 정당성을 완벽히 증명해야 하네.

   2. Safe Round Transition:
      - nextRound 결정 시 (votedRound + 1)과 현재 노드의 Round 사이의 정합성을 검토하게나.
      - 지연된 네트워크에서 '너무 과거'의 라운드로 제안되지 않도록
        nextRound = max(votedRound + 1, currentRound) 등의 방어 로직 도입을 고려하게.

   3. Signature Aggregation (Scalability):
      - 현재의 map[int][]byte 구조는 노드가 늘어날수록 QC가 비대해지네.
      - BLS Threshold Signature를 도입하여 수백 개의 서명을 고정 크기로 압축하고,
        Bitmask로 서명자 목록을 최적화하여 네트워크 대역폭을 사수하게나.

   4. Async Broadcast Pipe:
      - 노드 확장성에 대비하여 Broadcaster에 비동기 큐나 고루틴 풀을 적용하고,
        Gossip 프로토콜과의 연계로 전파 효율을 극대화할 준비를 하게나.
*/

package consensus

import (
	"arachnet-bft/network"
	"arachnet-bft/types"
	"sync"
	"time"
)

type Proposer struct {
	mu            sync.Mutex
	ctx           types.ConsensusContext
	QuorumManager QuorumPolicy
	Broadcaster   network.Broadcaster
}

func NewProposer(ctx types.ConsensusContext, qp QuorumPolicy, broadcaster network.Broadcaster) *Proposer {
	return &Proposer{
		ctx:           ctx,
		QuorumManager: qp,
		Broadcaster:   broadcaster,
	}
}

// Propose: $2f+1$ 부모를 확인하고 새 Vertex를 만들어 세상에 알리네!
func (p *Proposer) Propose() {

	// 1. 투표 수집 (Invariant: Validity)
	// Validator의 VotePool에서 현재 라운드에 대한 2f+1 투표를 가져오네.
	// 1. 투표 수집 시, 해당 투표들이 모인 '기준 라운드' 수집
	quorumVotes, votedRound := p.ctx.GetReadyQuorum()
	if quorumVotes == nil {
		return
	}

	// 2. 중복 제안 방지 (Invariant: Safety)
	currentRound := p.ctx.GetCurrentRound()
	nextRound := votedRound + 1
	if p.ctx.AlreadyProposed(nextRound) {
		return
	}

	// 3. 증거(QC)와 부모(Parents) 조립
	qc := p.assembleQC(quorumVotes, currentRound)
	parentHashes := p.extractHashesFromVotes(quorumVotes)

	// 4. 새 Vertex 생성
	vtx := &types.Vertex{
		Author:    p.ctx.GetID(),
		Round:     nextRound,
		Parents:   parentHashes,
		ParentQCs: []*types.QC{qc},
		Timestamp: time.Now().UnixMilli(),
		Payload:   p.fetchTransactions(),
	}

	// 5. 결정론적 해싱 및 서명
	vtx.Hash = vtx.CalculateHash()
	vtx.Signature = p.ctx.Sign(vtx.Hash)

	// 6. 전파 및 기록 [cite: 314]
	p.Broadcaster.Broadcast(types.MsgVertex, vtx)
	p.ctx.MarkAsProposed(nextRound)

	// 여기서 라운드를 올리지 마세!
	// "내가 제안한 이 Vertex가 남들에게 인정받아 다시 투표가 모일 때" 그때 올라가는 걸세.
}

// extractHashesFromVotes: 투표 묶음에서 중복을 제거한 부모 해시 목록을 추출하네.
func (p *Proposer) extractHashesFromVotes(votes []*types.Message) []string {
	hashSet := make(map[string]struct{})
	for _, msg := range votes {
		if msg.Vote != nil {
			hashSet[msg.Vote.VertexHash] = struct{}{}
		}
	}

	hashes := make([]string, 0, len(hashSet))
	for h := range hashSet {
		hashes = append(hashes, h)
	}
	return hashes
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

// assembleQC: 수집된 투표들로부터 정족수 증명(QC)을 조립하네.
func (p *Proposer) assembleQC(votes []*types.Message, round int) *types.QC {
	signatures := make(map[int][]byte)
	var representativeHash string

	for _, msg := range votes {
		if msg.Vote != nil {
			signatures[msg.FromID] = msg.Vote.Signature
			// 현재는 루프의 마지막 해시를 대표값으로 쓰지만, 이는 과도기적 구현일세.
			representativeHash = msg.Vote.VertexHash
		}
	}

	/* #TODO: [Scalability & Integrity Upgrade Path]

	   1. 다중 부모 해시 처리 (Multi-Hash Consensus):
	      - 현재는 하나의 representativeHash만 담고 있지만, DAG 기반에서는 2f+1개의 투표가
	        서로 다른 부모 해시를 가리킬 수 있네.
	      - 미래에는 이 해시들의 집합(Set)이나 머클 루트(Merkle Root)를 QC에 담아
	        모든 부모의 정당성을 동시에 증명해야 하네.

	   2. BLS Threshold Signature 도입:
	      - 현재의 map[int][]byte 구조는 노드 수가 늘어날수록 QC의 크기가 선형적으로 증가하네.
	      - 여기에 BLS 서명 결합(Aggregation)을 도입하여, 수백 개의 서명을 단 하나의
	        고정 크기 서명으로 압축해야 하네. (Network BW 절감의 핵심!)

	   3. Bitmask 최적화:
	      - 누가 서명했는지 map으로 기록하는 대신, 비트마스크를 사용하여 QC 헤더 크기를
	        획기적으로 줄이는 작업이 병행되어야 하네.
	*/

	return &types.QC{
		Type:       types.QCGlobal,
		VertexHash: representativeHash,
		Round:      round,
		ProposerID: p.ctx.GetID(),
		Signatures: signatures,
	}
}
