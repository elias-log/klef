//메시지 라우팅 (Voter, Dag, Fetcher로 전달)

/*
   <Validator Invariants>

   [Integrity] Trusted Initialization:
       Validator의 모든 하위 구성 요소(DAG, Fetcher, Slasher 등)는
       Config에 정의된 동일한 보안 정책과 리소스 한도를 공유해야 하네.

   [Safety] Double-Validation Defense:
       모든 인바운드 메시지는 로직 내부로 침투하기 전,
       반드시 preValidate(Strategy Pattern) 관문을 통과하여 데이터 무결성이 입증되어야 하네.

   [Consistency] Pending Request Singularity:
       임의의 Hash에 대해 '유효한(Active) 요청'은 시스템 내에 오직 하나만 존재해야 하며,
       ExpiryTime을 기준으로 맵(진실)과 큐(인덱스)의 정합성이 상시 유지되어야 하네.

   [Liveness] Recursive Freedom:
       Fetcher와 DAG 간의 상호작용(AddVertex -> StartSync)은
       어떠한 상황에서도 무한재귀나 데드락에 빠지지 않도록 비동기/이벤트 기반으로 격리되어야 하네.

   [Resource] Channel Backpressure:
       InboundMsg 채널은 Config에 설정된 버퍼 크기를 엄수하며,
       처리 속도가 유입 속도를 따라가지 못할 경우 시스템 전체의 안전을 위해
       정의된 정책(Drop or Block)에 따라 일관되게 행동해야 하네.
*/

// TODO: [Retry Policy & System Invariant]
//
// 1. 재시도 메커니즘 (Next Step):
//    - 현재 startPendingCleanup은 만료된 항목을 '제거'만 수행함.
//    - 의도: 제거 시점에 해당 Hash를 Fetcher나 별도의 RetryBuffer에 넘겨
//      네트워크에 재요청(Re-dispatch)을 보내는 로직이 연계되어야 함.
//
// 2. 시스템 불변성(Invariant) 보장:
//    - 하나의 Hash에 대해 "현재 유효한(Active) 요청은 반드시 최대 1개"여야 함.
//    - 모든 상태 판단은 ExpiryTime을 절대적 기준으로 삼음 (RequestTime은 기록용).
//    - 큐(PriorityQueue)와 맵(Map)의 데이터 정합성이 깨질 경우, 항상 맵(진실)을 기준으로 큐를 정리함.
//
// 3. 멱등성(Idempotency):
//    - 네트워크 지연으로 인한 중복 응답은 DAG.AddVertex의 존재 확인 로직에서
//      자연스럽게 필터링되도록 설계됨 (At-least-once delivery 수용).

package core

import (
	"arachnet-bft/config"
	"arachnet-bft/consensus"
	"arachnet-bft/core/validation"
	"arachnet-bft/types"
	"context"
	"fmt"
	"sync"
)

type Validator struct {
	// [Group 1: Identity & Static]
	ID        int
	PublicKey types.PublicKey
	Config    *config.Config

	// [Group 2: Core Components / Dependencies]
	Signer            types.Signer // 서명용 툴일세 e.g.,Ed25519Signer, BLSSigner
	DAG               *DAG
	Fetcher           *VertexFetcher
	Slasher           *Slasher
	messageValidators map[types.MessageType]validation.MessageValidator // Starategy Pattern: 메시지 타입별 검증기 보관함일세!
	Proposer          *consensus.Proposer
	Policy            consensus.QuorumPolicy

	// [Group 3: Consensust State & Peer Management]
	Round             int
	lastProposedRound int
	proposalMu        sync.Mutex
	PeerRounds        map[int]int  // 노드 ID -> 해당 노드가 알려준 최신 라운드
	peersMu           sync.RWMutex //
	Peers             map[int]bool // 현재 연결된 피어들의 ID 저장소 (ID -> 활성화 여부)

	// [Group 4: Async Task & Buffer Management]
	pendingMu       sync.RWMutex           // pending 맵과 큐를 동시에 보호
	pendingRequests map[string]PendingMeta // key: PeerID-RequestID 혹은 Hash, value: 요청 시간
	pendingQueue    PriorityQueue          // 시간 작을수록 루트에 가까움
	votePool        *VotePool              // 투표를 임시 저장

	// [Group 5: System Control & Networking]
	ctx        context.Context     // 종료 신호용
	cancel     context.CancelFunc  // 종료 함수
	InboundMsg chan *types.Message // 외부에서 메시지 수신
}

func NewValidator(id int, cfg *config.Config, signer types.Signer) *Validator {
	// 1. 기본 필드 및 맵/채널 초기화
	v := &Validator{
		ID:                id,
		Config:            cfg,
		Signer:            signer,
		PublicKey:         signer.GetPublicKey(),
		lastProposedRound: -1,
		Peers:             make(map[int]bool),
		PeerRounds:        make(map[int]int),
		pendingRequests:   make(map[string]PendingMeta),
		votePool:          NewVotePool(),
		messageValidators: make(map[types.MessageType]validation.MessageValidator),
		InboundMsg:        make(chan *types.Message, cfg.Resource.ValidatorChannelSize),
	}

	v.Policy = consensus.NewDynamicQuorumPolicy(
		4, // cfg.Consensus.TotalNodes 등으로 교체하게나
		4, // cfg.Consensus.CommitteeNodes 등으로 교체하게나
		cfg.Consensus.GlobalQuorumRatio,
		cfg.Consensus.CommitteeQuorumRatio,
	)
	v.Proposer = consensus.NewProposer(v, v.Policy, v)
	v.Slasher = NewSlasher(cfg)
	v.Fetcher = &VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.Resource.FetcherChannelSize),
		Validator:       v,
	}
	v.DAG = NewDAG(v.Fetcher, cfg)
	v.registerValidators()
	return v
}

func (v *Validator) registerValidators() {
	v.messageValidators[types.MsgFetchReq] = &validation.FetchRequestValidator{
		MaxRequestHashes: v.Config.Request.MaxFetchRequestHashes,
	}
	v.messageValidators[types.MsgFetchRes] = &validation.FetchResponseValidator{
		MaxVertexCount: v.Config.Request.MaxFetchResponseVtx,
	}
}

// Start: Validator의 모든 엔진에 시동을 거네!
func (v *Validator) Start(ctx context.Context) {
	v.ctx, v.cancel = context.WithCancel(ctx)

	// 1. 장부 정리 고루틴 실행
	go v.startPendingCleanup()

	// 2. 메시지 라우팅 루프 실행
	go func() {
		fmt.Printf("[SYSTEM] Validator %d: Message routing loop started.\n", v.ID)
		for {
			select {
			case msg := <-v.InboundMsg:
				v.routeMessage(msg)
			case <-v.ctx.Done():
				fmt.Printf("[SYSTEM] Validator %d: Shutting down...\n", v.ID)
				return
			}
		}
	}()
}

// Stop: 안전하게 시스템을 멈추네.
func (v *Validator) Stop() {
	v.cancel()
}

// [ValidatorContext 인터페이스 구현부]
func (v *Validator) GetCurrentRound() int {
	return v.Round
}

func (v *Validator) IsKnownVertex(hash string) bool {
	return v.DAG.GetVertex(hash) != nil
}

func (v *Validator) Sign(data string) types.Signature {
	// string 해시를 바이트로 변환해서 서명 대리인에게 맡기네.
	return v.Signer.Sign([]byte(data))
}

func (v *Validator) GetQuorumPolicy() consensus.QuorumPolicy {
	return v.Policy
}

func (v *Validator) QuorumThreshold() int {
	return v.Policy.GetQuorumSize(types.QCGlobal)
}

// Proposer가 쓸 수 있게 ID 반환 메서드 추가
func (v *Validator) GetID() int {
	return v.ID
}

// AlreadyProposed: 내가 이 라운드(혹은 그 이전)에 이미 제안했는지 확인하네.
func (v *Validator) AlreadyProposed(round int) bool {
	v.proposalMu.Lock()
	defer v.proposalMu.Unlock()
	return round <= v.lastProposedRound
}

// MarkAsProposed: 제안이 성공적으로 전파되었음을 장부에 기록하네.
func (v *Validator) MarkAsProposed(round int) {
	v.proposalMu.Lock()
	defer v.proposalMu.Unlock()
	if round > v.lastProposedRound {
		v.lastProposedRound = round
	}
}
