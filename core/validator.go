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
	"arachnet-bft/core/validation"
	"arachnet-bft/types"
	"container/heap"
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
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

	// [Group 3: Consensust State & Peer Management]
	Round      int
	PeerRounds map[int]int  // 노드 ID -> 해당 노드가 알려준 최신 라운드
	peersMu    sync.RWMutex //
	Peers      map[int]bool // 현재 연결된 피어들의 ID 저장소 (ID -> 활성화 여부)

	// [Group 4: Async Task & Buffer Management]
	pendingMu       sync.RWMutex           // pending 맵과 큐를 동시에 보호
	pendingRequests map[string]PendingMeta // key: PeerID-RequestID 혹은 Hash, value: 요청 시간
	pendingQueue    PriorityQueue          // 시간 작을수록 루트에 가까움

	// [Group 5: System Control & Networking]
	ctx        context.Context     // 종료 신호용
	cancel     context.CancelFunc  // 종료 함수
	InboundMsg chan *types.Message // 외부에서 메시지 수신

	// 네트워크 인터페이스를 여기에 연결해야 하네
	// network.Sender 인터페이스 등을 추가할 예정이네
}

func NewValidator(id int, cfg *config.Config, signer types.Signer) *Validator {
	// 1. 기본 필드 및 맵/채널 초기화
	v := &Validator{
		ID:        id,
		Config:    cfg,
		Signer:    signer,
		PublicKey: signer.GetPublicKey(),

		Peers:             make(map[int]bool),
		PeerRounds:        make(map[int]int),
		pendingRequests:   make(map[string]PendingMeta),
		messageValidators: make(map[types.MessageType]validation.MessageValidator),
		InboundMsg:        make(chan *types.Message, cfg.Resource.ValidatorChannelSize),
	}

	// 2. 하위 엔진(Components) 생성 및 의존성 주입 (Dependency Injection)
	v.Slasher = NewSlasher(cfg)

	// Fetcher 생성 (Validator 참조 주입)
	v.Fetcher = &VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.Resource.FetcherChannelSize),
		Validator:       v,
	}

	// DAG 생성 (Fetcher를 인터페이스로 주입)
	v.DAG = NewDAG(v.Fetcher, cfg)

	// 3. 메시지 검증 전략(Strategy) 등록
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

// Item: 큐에서 관리할 요청 정보
type PendingItem struct {
	Hash        string
	RequestTime time.Time // Monotonic Clock차이 해결용. 맵에 저장된 시간과 1:1 매칭할 녀석일세.
	ExpiryTime  time.Time
	index       int // heap 내부 관리를 위한 인덱스
}

type PendingMeta struct {
	RequestTime time.Time
	ExpiryTime  time.Time
	RetryCount  int
}

// PriorityQueue
type PriorityQueue []*PendingItem

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].ExpiryTime.Before(pq[j].ExpiryTime) }
func (pq PriorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i]; pq[i].index = i; pq[j].index = j }
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PendingItem)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// [ValidatorContext 인터페이스 구현부]
func (v *Validator) GetCurrentRound() int {
	return v.Round
}

func (v *Validator) IsKnownVertex(hash string) bool {
	return v.DAG.GetVertex(hash) != nil
}

// AddPeer: 새로운 피어가 연결되었을 때 불러주게나.
func (v *Validator) AddPeer(id int) {
	v.peersMu.Lock()
	defer v.peersMu.Unlock()
	v.Peers[id] = true
}

// getActivePeerIDs: 현재 살아있는 피어들의 ID만 슬라이스로 뽑아내네.
func (v *Validator) getActivePeerIDs() []int {
	v.peersMu.RLock()
	defer v.peersMu.RUnlock()

	ids := make([]int, 0, len(v.Peers))
	for id, active := range v.Peers {
		if active {
			ids = append(ids, id)
		}
	}
	return ids
}

func (v *Validator) GetRandomPeers(n int) []int {
	// 1. 현재 연결된 모든 피어 리스트를 가져오네 (가정)
	allPeers := v.getActivePeerIDs()
	totalActive := len(allPeers)

	// 2. 요청한 n보다 활성 노드가 적거나 같으면,
	if totalActive <= n {
		// 더 고민할 것 없이 현재 있는 피어를 다 반환하면 되네.
		// 슬라이스를 복사해서 주는 게 안전하네 (원본 보호!)
		result := make([]int, totalActive)
		copy(result, allPeers)
		return result
	}

	// 3. 노드가 충분하다면, 그중 n개만 무작위로 추출하세.
	return v.shuffleAndPick(allPeers, n)
}

func (v *Validator) shuffleAndPick(peers []int, n int) []int {
	// 슬라이스 원본이 상하지 않게 복사본을 만듦세.
	shuffled := make([]int, len(peers))
	copy(shuffled, peers)

	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return shuffled[:n]
}

// IsRequestPending: [TODO] 요청 장부(Pending Map)를 확인하는 로직이 들어갈 자리일세.
// 나중에 pendingRequests 맵을 도입하면 여기서 실제로 확인하게 될 걸세.
func (v *Validator) IsRequestPending(hash string) bool {
	v.pendingMu.RLock()
	defer v.pendingMu.RUnlock() // 함수가 끝날 때까지 안전하게 보호하세.

	meta, exists := v.pendingRequests[hash]
	if !exists {
		return false
	}

	// 만료 여부만 판단해서 알려주게나.
	// 별도의 고루틴 startPendingCleanup()으로 처리해서 리드만 해도 되게 처리하겠네.
	return time.Now().Before(meta.ExpiryTime)
}

// Validator 시작 시 한 번 띄워두는 녀석일세.
func (v *Validator) startPendingCleanup() {
	// 1. Ticker Leak 방지: 안해두면 죽은 애가 RequestTimeout의 최대 2배까지 살아남네.
	checkInterval := v.Config.Request.BaseTimeout / 2
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			v.pendingMu.Lock()
			now := time.Now()

			// 2. Priority Queue 활용: 만료된 녀석들만 쏙쏙 골라내기
			// 맵 전체 순회(O(N)) 없이 큐의 앞부분(O(log N))만 확인하네.
			for v.pendingQueue.Len() > 0 {
				// 가장 오래된 요청 확인
				item := v.pendingQueue[0]
				meta, exists := v.pendingRequests[item.Hash] // 이제 meta는 PendingMeta 타입일세!

				// 맵에 저장된 시간과 큐에 들어있던 생성 시간이 다르면 '가짜' 혹은 '낡은' 데이터일세.
				if !exists || !meta.RequestTime.Equal(item.RequestTime) {
					heap.Pop(&v.pendingQueue)
					continue
				}

				// 제일 오래된 놈도 아직 안 죽었으면 나머지도 다 살아있네!
				if now.Before(item.ExpiryTime) {
					break
				}

				// 만료된 녀석 제거
				heap.Pop(&v.pendingQueue)
				delete(v.pendingRequests, item.Hash)
			}
			v.pendingMu.Unlock()

		case <-v.ctx.Done():
			// 3. Validator 종료 시 고루틴 종료
			return
		}
	}
}

// AddPendingRequest: 맵(진실)을 갱신하고 큐(참고용)에 이정표를 남기네.
// AddPendingRequest: 지수 백오프와 지터가 적용된 방탄 요청 추가 로직일세.
func (v *Validator) AddPendingRequest(hash string) {
	v.pendingMu.Lock()
	defer v.pendingMu.Unlock()

	now := time.Now()
	meta, exists := v.pendingRequests[hash]

	// 2. [중복 체크] 이미 유효한 요청이 진행 중이면 무시
	if exists && now.Before(meta.ExpiryTime) {
		return
	}

	// 3. 리트라이 횟수 계산
	retryCount := 0
	if exists {
		retryCount = meta.RetryCount + 1
	}

	// 4. 지수 백오프 및 지터 계산
	// backoff overflow 방지 / MaxBackoff 적용
	backoff := v.Config.Request.BaseTimeout * time.Duration(1<<retryCount)
	if backoff > v.Config.Request.MaxBackoff {
		backoff = v.Config.Request.MaxBackoff
	}

	// Jitter 계산: 0 ~ backoff/5 사이의 무작위 값
	jitter := time.Duration(0)
	if backoff > 0 {
		jitter = time.Duration(rand.Int63n(int64(backoff / 5)))
	}

	expiry := now.Add(backoff + jitter)

	// 5. map(진실) 갱신 & Queue() 삽입
	v.pendingRequests[hash] = PendingMeta{
		RequestTime: now,
		ExpiryTime:  expiry,
		RetryCount:  retryCount,
	}

	heap.Push(&v.pendingQueue, &PendingItem{
		Hash:        hash,
		RequestTime: now,
		ExpiryTime:  expiry,
	})
}

// 공통 검증 진입점: handle 함수들이 호출하기 전에 거쳐가는 관문이라네.
func (v *Validator) preValidate(msg *types.Message) error {
	// 이제 MessageValidator는 validation 패키지에 있네!
	if val, ok := v.messageValidators[msg.Type]; ok {
		// 여기서 v(Validator)는 ValidatorContext 인터페이스로 전달되네!
		return val.Validate(msg, v)
	}
	// 등록되지 않은 메시지 타입에 대한 처리
	return fmt.Errorf("unknown message type: %s", msg.Type)
}

func (v *Validator) UpdatePeerRound(peerID int, round int) {
	if round > v.PeerRounds[peerID] {
		v.PeerRounds[peerID] = round
	}
}

// f.Validator.SendTo(suspectID, req) 해결
func (v *Validator) SendTo(peerID int, msg *types.Message) {
	// 실제 전송 로직 (나중에 네트워크 레이어와 연결할 부분일세)
}

// f.Validator.Broadcast(req) 해결
func (v *Validator) Broadcast(msg *types.Message) {
	// 모든 피어에게 SendTo를 호출하는 반복문이 들어갈 걸세
}

func (v *Validator) handleFetchRequest(msg *types.Message) {
	// 1. 관문 통과!
	if err := v.preValidate(msg); err != nil {
		fmt.Printf("검증 실패(Req): %v\n", err)
		return
	}

	// 2. 페이로드에서 요청된 해시 목록을 꺼내네.
	req := msg.Payload.(types.FetchRequest)
	var foundVertices []*types.Vertex

	// 3. 내 DAG를 뒤져서 있는 것만 골라 담게나.
	for _, h := range req.MissingHashes {
		if vtx := v.DAG.GetVertex(h); vtx != nil {
			foundVertices = append(foundVertices, vtx)
		}
	}

	// 4. 찾은 게 있다면 답장을 보내야지!
	if len(foundVertices) > 0 {
		response := &types.Message{
			FromID:       v.ID,
			CurrentRound: v.Round,
			Type:         types.MsgFetchRes,
			Payload:      types.FetchResponse{Vertices: foundVertices},
		}
		v.SendTo(msg.FromID, response)
	}
}

func (v *Validator) handleFetchResponse(msg *types.Message) {

	// 1. 관문 통과!
	if err := v.preValidate(msg); err != nil {
		fmt.Printf("검증 실패(Res): %v\n", err)
		return
	}

	// 2. 페이로드에서 Vertex 뭉치를 꺼내네.
	res := msg.Payload.(types.FetchResponse)

	// 3. 받은 Vertex들을 하나씩 DAG에 삽입 시도하네.
	for _, vtx := range res.Vertices {
		// 이미 DAG에 있는지 확인 (중복 삽입 방지)
		if v.DAG.GetVertex(vtx.Hash) != nil {
			continue
		}

		// 4. 정식 삽입 로직 호출!
		// 이 안에서 Buffer를 확인하고 자식들을 깨우는 도미노가 시작될 걸세.
		// 이미 preValidate에서 기본적인 데이터 무결성은 확인했으니 안심하고 넣네!
		v.DAG.AddVertex(vtx, v.Round)
	}
}

func (v *Validator) Sign(data string) types.Signature {
	// string 해시를 바이트로 변환해서 서명 대리인에게 맡기네.
	return v.Signer.Sign([]byte(data))
}
