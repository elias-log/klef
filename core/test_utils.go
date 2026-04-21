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

	// 1. 먼저 해시를 계산하네.
	vtx.Hash = vtx.CalculateHash()

	// 2. 계산된 해시에 서명을 입히네.
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
	hashes := make([]string, 0, count)

	// 1. 제네시스 정점
	genesis := &types.Vertex{
		Author:    0,
		Round:     0,
		Parents:   []string{}, // 부모 없음
		Timestamp: time.Now().UnixNano(),
	}
	genesis.Hash = genesis.CalculateHash()

	vertices = append(vertices, genesis)
	hashes = append(hashes, genesis.Hash)

	for i := 1; i < count; i++ {
		// 2. 인과관계에 따른 부모 선택
		available := len(hashes)
		numParents := rand.Intn(3) + 1
		if numParents > available {
			numParents = available // 가질 수 있는 만큼만
		}

		parents := make([]string, 0, numParents)
		parentSet := make(map[string]struct{}) // 중복 부모 방지를 위한 맵

		attempts := 0 // 무한 루프 방지용 카운터
		for len(parents) < numParents && attempts < 100 {
			attempts++
			pHash := hashes[rand.Intn(len(hashes))]
			if _, exists := parentSet[pHash]; !exists {
				parentSet[pHash] = struct{}{}
				parents = append(parents, pHash)
			}
		}

		// 부모 해시들도 정렬해주면 더 결정론적이겠지?
		sort.Strings(parents)

		vtx := &types.Vertex{
			Author:    i % 4,
			Round:     i / 4,
			Parents:   parents,
			Timestamp: time.Now().UnixNano() + int64(i), // 미세한 시간 차이
			Payload:   [][]byte{[]byte(fmt.Sprintf("tx-data-%d", i))},
		}

		vtx.Hash = vtx.CalculateHash()

		vertices = append(vertices, vtx)
		hashes = append(hashes, vtx.Hash)
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
