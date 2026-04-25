// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package core

import (
	"fmt"
	"klef/config"
	"klef/pkg/types"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func NewTestValidator(t *testing.T, cfg *config.Config, signer *Ed25519Signer) *Validator {
	t.Helper()

	v := NewValidator(0, cfg, signer)

	slasher := NewSlasher(cfg)

	fetcher := &VertexFetcher{
		InboundResponse: make(chan *types.Vertex, cfg.Resource.FetcherChannelSize),
	}

	dag := NewDAG(fetcher, slasher, cfg)

	v.Slasher = slasher
	v.DAG = dag

	if v.DAG == nil {
		t.Fatal("DAG is nil")
	}

	return v
}

/// CreateDummyVertex generates a standard, valid test vertex with specified metadata and parents.
func CreateDummyVertex(author int, round int, parents []types.Hash, signer *Ed25519Signer) *types.Vertex {
	vtx := &types.Vertex{
		Author:  author,
		Round:   round,
		Parents: parents,
		ParentQCs: []*types.QC{{
			Type:       types.QCGlobal,
			Round:      round - 1,    // Parents usually reside in the preceding round.
			VertexHash: types.Hash{}, // Dummy parent hash for testing.
		}},
		Timestamp: time.Now().UnixMilli(),
		Payload:   [][]byte{[]byte("test_transaction")},
	}

	// Ensure compliance with protocol rules.
	vtx.Normalize()

	// Generate the canonical hash.
	vtx.Hash = vtx.CalculateHash()

	// Apply cryptographic signature if a signer is provided.
	if signer != nil {
		sig := signer.Sign(vtx.Hash.Bytes())
		vtx.Signature = sig
	}

	return vtx
}

/// CreateByzantineVertex generates an intentionally malformed vertex to test validation logic.
/// It skips normalization to simulate rule violations like unsorted parents or invalid hashes.
func CreateByzantineVertex(author int, round int, parents []types.Hash, signer *Ed25519Signer) *types.Vertex {
	vtx := &types.Vertex{
		Author:    author,
		Round:     round,
		Parents:   parents, // Input parents are preserved without normalization/sorting.
		Timestamp: time.Now().UnixMilli(),
	}

	// Hash is calculated on malformed data to test signature/hash mismatch detection.
	vtx.Hash = vtx.CalculateHash()

	if signer != nil {
		sig := signer.Sign(vtx.Hash.Bytes())
		vtx.Signature = sig
	}
	return vtx
}

/// InjectFetchResponse manually pushes a simulated FetchResponse message into a validator's ingress.
func InjectFetchResponse(r interface {
	RouteMessage(*types.Message)
}, vertices []*types.Vertex) {
	msg := &types.Message{
		FromID: 999,
		Type:   types.MsgFetchRes,
		FetchRes: &types.FetchResponse{
			Vertices: vertices,
		},
	}

	r.RouteMessage(msg)
}

/// generateComplexDAG constructs a multi-layered set of vertices with randomized causal links.
func generateComplexDAG(count int) []*types.Vertex {
	vertices := make([]*types.Vertex, 0, count)

	// Initialize with a Genesis vertex.
	genesis := &types.Vertex{
		Author:    0,
		Round:     0,
		Parents:   []types.Hash{}, // no parent
		Timestamp: time.Now().UnixNano(),
	}
	genesis.Normalize()
	genesis.Hash = genesis.CalculateHash()
	vertices = append(vertices, genesis)

	for i := 1; i < count; i++ {
		// Select parents only from previously finalized vertices in the slice.
		numParents := rand.Intn(3) + 1
		if numParents > len(vertices) {
			numParents = len(vertices)
		}

		var parents []types.Hash
		parentSet := make(map[types.Hash]struct{})

		for len(parents) < numParents {
			p := vertices[rand.Intn(len(vertices))]

			if _, exists := parentSet[p.Hash]; !exists {
				parentSet[p.Hash] = struct{}{}
				parents = append(parents, p.Hash)
			}
		}

		types.SortHashes(parents)

		vtx := &types.Vertex{
			Author:    i % 4,
			Round:     i / 4,
			Parents:   parents,
			Timestamp: time.Now().UnixNano() + int64(i),
			Payload:   [][]byte{[]byte(fmt.Sprintf("tx-data-%d", i))},
		}

		vtx.Normalize()
		vtx.Hash = vtx.CalculateHash()
		vertices = append(vertices, vtx)
	}
	return vertices
}

/// shuffle returns a new slice containing the same vertices in a randomized order.
func shuffle(src []*types.Vertex) []*types.Vertex {
	dest := make([]*types.Vertex, len(src))
	copy(dest, src)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(dest), func(i, j int) {
		dest[i], dest[j] = dest[j], dest[i]
	})

	return dest
}

/// compareDAG verifies if two DAG instances share the same logical structure by comparing Tips.
func compareDAG(dagA, dagB *DAG) bool {
	if dagA.Size() != dagB.Size() {
		return false
	}

	tipsA := dagA.GetTips()
	tipsB := dagB.GetTips()

	if len(tipsA) != len(tipsB) {
		return false
	}

	return reflect.DeepEqual(sortTips(tipsA), sortTips(tipsB))
}

/// sortTips extracts and lexicographically sorts hashes from a vertex slice for comparison.
func sortTips(tips []*types.Vertex) []types.Hash {
	hashes := make([]types.Hash, len(tips))
	for i, v := range tips {
		hashes[i] = v.Hash
	}
	types.SortHashes(hashes)
	return hashes
}

/// compareDAGFull performs a deep exhaustive comparison of every vertex and its properties.
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
