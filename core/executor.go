//앵커 선정 및 합의 순서 결정 (Linearization)
//DAG를 탐색하여 합의된 순서를 결정하고, 상위 앱으로 트랜잭션을 전달함.

package core

// Executor는 DAG를 감시하며 확정 로직을 수행하네.
type Executor struct {
	// 확정된 마지막 라운드 번호
	LastCommittedRound int
}

// CommitLogic: 앵커를 선택하고 경로를 탐색하여 순서를 확정하네.
func (e *Executor) CommitLogic(dag *DAG) {
	// 1. 현재 라운드가 짝수인지 확인 (Bullshark 기준)
	// 2. 앵커(Leader) Vertex 선정
	// 3. 2f+1 연결 확인 후, 경로상의 모든 Vertex를 리스트화
	// 4. 리스트를 트랜잭션 실행기로 전달
}
