package core

import (
	"arachnet-bft/types"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"time"
)

// CreateDummyVertex: 라운드와 부모를 지정하면 테스트용 Vertex를 만들어주네.
func CreateDummyVertex(author int, round int, parents []string, signer *Ed25519Signer) *types.Vertex {
	vtx := &types.Vertex{
		Author:  author,
		Round:   round,
		Parents: parents,
		ParentQCs: []*types.QC{{
			Type:       types.QCGlobal,
			Round:      round - 1, // 보통 부모는 이전 라운드니까 말이네.
			VertexHash: "dummy_parent_hash",
		}},
		Timestamp: time.Now().UnixMilli(),
		Payload:   [][]byte{[]byte("test_transaction")},
	}

	vtx.Normalize()

	// 1. 먼저 해시를 계산하네.
	vtx.Hash = vtx.CalculateHash()

	// 2. 계산된 해시에 서명을 입히네.
	if signer != nil {
		sig := signer.Sign([]byte(vtx.Hash))
		vtx.Signature = []byte(sig)
	}

	return vtx
}

func CreateByzantineVertex(author int, round int, parents []string, signer *Ed25519Signer) *types.Vertex {
	vtx := &types.Vertex{
		Author:    author,
		Round:     round,
		Parents:   parents, // 정렬 안 된 그대로 넣음!
		Timestamp: time.Now().UnixMilli(),
	}
	// Normalize()를 생략하고 바로 해시를 뜸! (이게 바로 의도적인 규칙 위반)
	vtx.Hash = vtx.CalculateHash()

	if signer != nil {
		sig := signer.Sign([]byte(vtx.Hash))
		vtx.Signature = []byte(sig)
	}
	return vtx
}

// InjectFetchResponse: 특정 검증인에게 가짜 FetchResponse를 주입하네.
func InjectFetchResponse(v *Validator, vertices []*types.Vertex) {
	msg := &types.Message{
		FromID:   999,
		Type:     types.MsgFetchRes,
		FetchRes: &types.FetchResponse{Vertices: vertices},
	}
	v.InboundMsg <- msg
}

// 1. generateComplexDAG: 복잡하게 꼬인 테스트용 Vertex 세트를 생성하네.
func generateComplexDAG(count int) []*types.Vertex {
	vertices := make([]*types.Vertex, 0, count)

	// 1. 제네시스 정점
	genesis := &types.Vertex{
		Author:    0,
		Round:     0,
		Parents:   []string{}, // 부모 없음
		Timestamp: time.Now().UnixNano(),
	}
	genesis.Normalize() // 습관적으로!
	genesis.Hash = genesis.CalculateHash()
	vertices = append(vertices, genesis)

	for i := 1; i < count; i++ {
		// [수정] 이미 완전히 생성되어 슬라이스에 들어간 vertex 중에서만 부모를 고르네.
		numParents := rand.Intn(3) + 1
		if numParents > len(vertices) {
			numParents = len(vertices)
		}

		var parents []string
		parentSet := make(map[string]struct{}) // 중복 부모 방지를 위한 맵

		// 랜덤하게 섞인 인덱스에서 부모 선택
		for len(parents) < numParents {
			// 이미 생성된 vertices 중에서 랜덤하게 부모 선택
			p := vertices[rand.Intn(len(vertices))]

			if _, exists := parentSet[p.Hash]; !exists {
				parentSet[p.Hash] = struct{}{}
				parents = append(parents, p.Hash)
			}
		}

		// 규격 준수
		sort.Strings(parents)

		vtx := &types.Vertex{
			Author:    i % 4,
			Round:     i / 4,
			Parents:   parents,
			Timestamp: time.Now().UnixNano() + int64(i), // 미세한 시간 차이
			Payload:   [][]byte{[]byte(fmt.Sprintf("tx-data-%d", i))},
		}

		vtx.Normalize()
		vtx.Hash = vtx.CalculateHash()

		vertices = append(vertices, vtx)
	}
	return vertices
}

// 2. shuffle: 슬라이스의 순서를 무작위로 뒤섞네.
func shuffle(src []*types.Vertex) []*types.Vertex {
	dest := make([]*types.Vertex, len(src))
	copy(dest, src)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(dest), func(i, j int) {
		dest[i], dest[j] = dest[j], dest[i]
	})

	return dest
}

// 3. compareDAG: 두 노드의 DAG가 논리적으로 완벽히 일치하는지 검사하네.
func compareDAG(dagA, dagB *DAG) bool {
	// 1. 전체 개수 비교
	if dagA.Size() != dagB.Size() {
		return false
	}

	// 2. Tips의 해시 세트 비교
	tipsA := dagA.GetTips() // map[string]*Vertex 혹은 []string 반환 가정
	tipsB := dagB.GetTips()

	if len(tipsA) != len(tipsB) {
		return false
	}

	// Tips의 해시를 정렬해서 비교
	return reflect.DeepEqual(sortTips(tipsA), sortTips(tipsB))
}

// sortTips: Vertex 슬라이스에서 해시만 추출하여 사전순으로 정렬하네.
func sortTips(tips []*types.Vertex) []string {
	hashes := make([]string, len(tips))
	for i, v := range tips {
		hashes[i] = v.Hash
	}
	sort.Strings(hashes)
	return hashes
}

func compareDAGFull(a, b *DAG) bool {
	if a.Size() != b.Size() {
		return false
	}

	for hash, vA := range a.Vertices {
		vB := b.GetVertex(hash)
		if vB == nil {
			return false
		}

		if !reflect.DeepEqual(vA.Parents, vB.Parents) {
			return false
		}

		if vA.Round != vB.Round {
			return false
		}
	}

	return true
}
