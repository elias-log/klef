package ds

type HeapItem interface {
	Less(other HeapItem) bool // Less: true면 힙의 앞쪽으로 가네.
	SetIndex(idx int)
	GetIndex() int
}

type PriorityQueue []HeapItem

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Less(pq[j]) }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].SetIndex(i)
	pq[j].SetIndex(j)
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(HeapItem)
	item.SetIndex(n)
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.SetIndex(-1)
	old[n-1] = nil // 메모리 누수 방지
	*pq = old[0 : n-1]
	return item
}
