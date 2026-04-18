# arachnet-bft

```
.
├── config/             # 시스템 정책 및 파라미터 관리 (Timeout, Threshold 등)
├── consensus/          # 합의 엔진: Proposer와 투표 집계(Vote Aggregator) 로직
├── core/               
│   ├── validation/     # 세부 검증 규칙 (Fetch/Sync 규정)
│   ├── dag.go          # Vertex 간의 인과 관계 및 그래프 관리
│   ├── fetcher.go      # 누락된 데이터를 찾아오는 사냥꾼
│   ├── slasher.go      # 부정 노드를 적발하고 처벌
│   └── validator.go    # 노드의 메인 제어 로직 (Network/Peer/Pending 관리)
├── network/            # 전송 계층: Broadcaster 및 직렬화(Codec) 담당
├── types/              # 공통 규격: Message, Vertex, Vote 등 핵심 데이터 구조
└── main.go             # Arachnet-BFT 노드 실행 엔트리 포인트
```
