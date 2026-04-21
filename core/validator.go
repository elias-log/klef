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

// [Updated]
// Validator는 시스템의 Orchestrator일세.
// 직접 로직을 수행하기보다 하위 컴포넌트들을 조율하는 역할에 집중하네.
type Validator struct {
	// [Group 1: Identity & Static]
	ID        int
	PublicKey types.PublicKey
	Config    *config.Config

	// [Group 2: Core Components / Dependencies]
	Signer            types.Signer
	DAG               *DAG
	Fetcher           *VertexFetcher
	Slasher           *Slasher
	messageValidators map[types.MessageType]validation.MessageValidator
	Proposer          *consensus.Proposer
	Policy            consensus.QuorumPolicy

	// [Group 3: Consensus State & Peer Management]
	Round             int
	lastProposedRound int
	proposalMu        sync.Mutex
	PeerRounds        map[int]int  // 노드 ID -> 해당 노드가 알려준 최신 라운드
	peersMu           sync.RWMutex //
	Peers             map[int]bool // 현재 연결된 피어들의 ID 저장소 (ID -> 활성화 여부)

	// [Group 4: Async Task & Buffer Management]
	pendingMgr *PendingManager
	votePool   *VotePool // 투표를 임시 저장

	// [Group 5: System Control & Networking]
	ctx        context.Context     // 종료 신호용
	cancel     context.CancelFunc  // 종료 함수
	InboundMsg chan *types.Message // 외부에서 메시지 수신
}

func NewValidator(id int, cfg *config.Config, signer types.Signer) *Validator {
	ctx, cancel := context.WithCancel(context.Background()) // [fix]

	// 1. 기본 필드 및 맵/채널 초기화
	v := &Validator{
		ID:                id,
		Config:            cfg,
		Signer:            signer,
		PublicKey:         signer.GetPublicKey(),
		lastProposedRound: -1,

		// [fix]
		ctx:    ctx,
		cancel: cancel,

		Peers:             make(map[int]bool),
		PeerRounds:        make(map[int]int),
		pendingMgr:        NewPendingManager(cfg),
		votePool:          NewVotePool(),
		messageValidators: make(map[types.MessageType]validation.MessageValidator),
		InboundMsg:        make(chan *types.Message, cfg.Resource.ValidatorChannelSize),
	}

	// 정책 및 엔진 초기화
	v.Policy = consensus.NewDynamicQuorumPolicy(
		cfg.Consensus.TotalNodes,
		cfg.Consensus.CommitteeNodes,
		cfg.Consensus.GlobalQuorumRatio,
		cfg.Consensus.CommitteeQuorumRatio,
	)
	v.Proposer = consensus.NewProposer(v, v.Policy, v)
	v.Slasher = NewSlasher(cfg)
	v.Fetcher = &VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.Resource.FetcherChannelSize),
		Validator:       v,
	}
	v.DAG = NewDAG(v.Fetcher, v.Slasher, cfg)

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
	go v.pendingMgr.StartCleanupLoop(v.ctx) // 펜딩 매니저 실행
	go v.runMessageLoop()                   // 메시지 라우팅 루프 실행
}

func (v *Validator) runMessageLoop() {
	fmt.Printf("[SYSTEM] Validator %d: Message routing loop started.\n", v.ID)

	for {
		select {
		case msg := <-v.InboundMsg:
			// routeMessage 로직은 나중에 분량에 따라 이동 고려
			v.routeMessage(msg)

		case <-v.ctx.Done():
			fmt.Printf("[SYSTEM] Validator %d: Shutting down...\n", v.ID)
			return
		}
	}
}

func (v *Validator) Stop() {
	v.cancel()
}
