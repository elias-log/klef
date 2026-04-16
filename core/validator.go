//메시지 라우팅 (Voter, Dag, Fetcher로 전달)

// TODO:
//
// [1] 시간 기준 통일 (가장 중요)
// - IsRequestPending / cleanup / retry 모두 같은 기준을 써야 함
// - 현재는 RequestTime 기준 vs ExpiryTime 기준이 섞여 있음 → 버그 위험
// - 해결:
//   → PendingMeta에 ExpiryTime 추가하고
//   → 모든 판단을 ExpiryTime 기준으로 통일
//
//   type PendingMeta struct {
//       RequestTime time.Time
//       RetryCount  int
//       ExpiryTime  time.Time
//   }
//
//   IsRequestPending:
//       return time.Now().Before(meta.ExpiryTime)
//
//
// [2] backoff overflow 방지
// - backoff = timeout * (1 << retryCount)
// - retryCount 커지면 duration overflow 가능
// - 해결:
//   → maxBackoff 제한 추가
//
//   maxBackoff := v.Config.RequestTimeout * 64
//   if backoff > maxBackoff {
//       backoff = maxBackoff
//   }
//
//
// [3] heap/map 일관성 유지 (이미 잘했지만 유지 중요)
// - map = 진실, heap = 인덱스
// - 항상 RequestTime 기준으로 stale 검증
//
//   if !exists || !meta.RequestTime.Equal(item.RequestTime) {
//       heap.Pop(...)
//       continue
//   }
//
//
// [4] pendingRequests 단일 진입점 유지
// - AddPendingRequest 외에서 map 직접 수정 금지
// - 구조 깨지는 주요 원인임
//
//
// [5] retry 정책 확인
// - 현재: timeout 지나면 재요청 + exponential backoff + jitter
// - 의도:
//   → 네트워크 지연 / 실패 상황에서도 안정적으로 재시도
//   → 중복 요청은 허용 (idempotent 처리 전제)
//
//
// [6] 시스템 invariant (절대 깨지면 안 되는 규칙)
// - 하나의 hash에 대해 “현재 유효한 요청은 최대 1개”
// - ExpiryTime 기준으로만 상태 판단
// - stale heap item은 반드시 제거됨
//
//

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
	ID                int            // f.Validator.ID 해결
	Config            *config.Config // 장부를 들고 다님세
	Round             int            // v.Round 해결
	DAG               *DAG           // f.Validator.DAG 해결
	Fetcher           *VertexFetcher
	Slasher           *Slasher
	Peers             map[int]bool                                      // 현재 연결된 피어들의 ID 저장소 (ID -> 활성화 여부)
	peersMu           sync.RWMutex                                      // 피어 목록 락!
	PeerRounds        map[int]int                                       // 노드 ID -> 해당 노드가 알려준 최신 라운드
	pendingRequests   map[string]PendingMeta                            // key: PeerID-RequestID 혹은 Hash, value: 요청 시간
	pendingQueue      PriorityQueue                                     // 시간순 정렬 큐
	pendingMu         sync.RWMutex                                      // 펜딩맵 락!
	ctx               context.Context                                   // 종료 신호용
	cancel            context.CancelFunc                                // 종료 함수
	messageValidators map[types.MessageType]validation.MessageValidator // Starategy Pattern: 메시지 타입별 검증기 보관함일세!
	Signer            types.Signer                                      // 서명용 툴일세 e.g.,Ed25519Signer, BLSSigner
	PublicKey         types.PublicKey                                   //

	// 네트워크 인터페이스를 여기에 연결해야 하네 (가칭)
	// 실제 구현 시에는 network.Sender 인터페이스 등을 추가할 예정이네
}

func NewValidator(id int, cfg *config.Config, signer types.Signer) *Validator {
	v := &Validator{
		ID:                id,
		Config:            cfg,
		Round:             0,
		Peers:             make(map[int]bool),
		PeerRounds:        make(map[int]int),
		pendingRequests:   make(map[string]PendingMeta),
		messageValidators: make(map[types.MessageType]validation.MessageValidator),
		Signer:            signer,
	}

	v.PublicKey = signer.GetPublicKey()
	v.Slasher = NewSlasher(cfg)

	// Fetcher -> Validator 연결
	v.Fetcher = &VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.FetcherChannelSize),
		Validator:       v,
	}

	// NewDAG를 호출할 때 Fetcher를 인자로 넘겨주면 되네!
	// NewDAG 내부에서는 이 인자를 SyncFetcher 인터페이스로 받겠지?
	// DAG -> Fetcher 연결 완료
	v.DAG = NewDAG(v.Fetcher, cfg.OrphanCapacity, cfg)

	// 4. 메시지 검증 Strategy 등록
	v.messageValidators[types.MsgFetchReq] = &validation.FetchRequestValidator{MaxRequestHashes: cfg.MaxFetchRequestHashes}
	v.messageValidators[types.MsgFetchRes] = &validation.FetchResponseValidator{MaxVertexCount: cfg.MaxFetchResponseVtx}

	return v
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
	// 여기서 지우려고 애쓰지 않아도 되네. (Read Lock 상태에선 지울 수도 없고 말이세!)
	// 별도의 고루틴 startPendingCleanup()으로 처리해서 리드만 해도 되게 처리하겠네.
	return time.Since(meta.RequestTime) <= v.Config.RequestTimeout
}

// Validator 시작 시 한 번 띄워두는 녀석일세.
func (v *Validator) startPendingCleanup() {
	// 1. Ticker Leak 방지: 안해두면 죽은 애가 RequestTimeout의 최대 2배까지 살아남네.
	checkInterval := v.Config.RequestTimeout / 2
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

	// 1. 기존 메타 정보 확인
	meta, exists := v.pendingRequests[hash]

	// 2. [중복 체크] 아직 대기 시간이 남았다면 중복 요청을 막으세.
	// (여기서의 '타임아웃'은 백오프가 적용된 이전의 expiry 기준일 수도 있지만,
	// 간단하게 RequestTime 기준으로만 체크해도 힙 폭발은 막을 수 있네.)
	if exists && time.Since(meta.RequestTime) < v.Config.RequestTimeout {
		return
	}

	// 3. 리트라이 횟수 계산
	retryCount := 0
	if exists {
		retryCount = meta.RetryCount + 1
		if retryCount > 6 {
			retryCount = 6
		}
	}

	// 4. 지수 백오프 및 지터 계산
	// 1<<retryCount는 1, 2, 4, 8, 16, 32, 64로 늘어나네.
	backoff := v.Config.RequestTimeout * time.Duration(1<<retryCount)

	// 지터(Jitter) 계산: 0 ~ backoff/5 사이의 무작위 값
	var jitter time.Duration
	if backoff > 0 {
		jitter = time.Duration(rand.Int63n(int64(backoff / 5)))
	}

	expiry := now.Add(backoff + jitter)

	// 5. 맵(진실) 갱신 - 이제 PendingMeta 타입을 넣으세!
	v.pendingRequests[hash] = PendingMeta{
		RequestTime: now,
		RetryCount:  retryCount,
	}

	// 6. 큐(참고용)에 삽입
	item := &PendingItem{
		Hash:        hash,
		RequestTime: now,
		ExpiryTime:  expiry,
	}
	heap.Push(&v.pendingQueue, item)
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
