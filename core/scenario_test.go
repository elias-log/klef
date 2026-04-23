// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package core

import (
	"klef/config"
	"klef/types"
	"math/rand"
	"sync"
	"testing"
	"time"
)

/// TestDAGOrphanAndInsertion verifies that a vertex with unknown parents is
/// correctly moved to the Orphan Buffer rather than being discarded or inserted.
func TestDAGOrphanAndInsertion(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// Simulate the current node being at Round 10.
	currentRound := 10

	// 1. Create an 'Orphan' Vertex (Round 5) with a non-existent parent hash.
	missingHash := types.Hash{0xde, 0xad, 0xbe, 0xef}
	orphanVtx := CreateDummyVertex(1, 5, []types.Hash{missingHash}, signer)

	// 2. Attempt to add the vertex. It should fail causal validation and enter the Buffer.
	v.DAG.AddVertex(orphanVtx, currentRound)

	// Verification: Ensure the vertex is residing in the Orphan Buffer.
	if found := v.DAG.Buffer.GetOrphan(orphanVtx.Hash); found == nil {
		t.Errorf("❌ Vertex %s failed to enter the Orphanage!", orphanVtx.Hash.String()[:8])
	} else {
		t.Logf("✅ Success: Vertex %s recognized as orphan and is awaiting parents.", orphanVtx.Hash.String()[:8])
	}
}

/// TestOrphanResolution checks if an orphan vertex is automatically moved
/// to the DAG once its missing parent is finally inserted.
func TestOrphanResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 1. Prepare the 'Real Parent' but do not insert it yet.
	parentVtx := CreateDummyVertex(0, 0, []types.Hash{}, signer)
	realParentHash := parentVtx.Hash

	// 2. Create an orphan that depends on the real parent hash.
	orphan := CreateDummyVertex(1, 1, []types.Hash{realParentHash}, signer)

	// 3. Inject the orphan. It should go to the buffer as the parent is missing.
	v.DAG.AddVertex(orphan, 1)

	if v.DAG.Buffer.GetOrphan(orphan.Hash) == nil {
		t.Fatal("Orphan failed to register in orphanage.")
	}

	// 4. Inject the 'Real Parent' to trigger resolution.
	v.DAG.AddVertex(parentVtx, 1)

	// 5. Final Verification: The orphan should have 'escaped' the buffer and joined the DAG.
	if v.DAG.GetVertex(orphan.Hash) != nil {
		t.Log("✅ Success! Orphan automatically resolved upon parent arrival.")
	} else {
		t.Errorf("❌ Orphan still in buffer despite parent being formally inserted.")
	}
}

/// TestDeepOrphanChain tests recursive orphan resolution (Domino Effect).
/// Grandma (v1) -> Mom (v2) -> Child (v3).
func TestDeepOrphanChain(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 1. Generation: v1 (Gen 0) -> v2 (Gen 1) -> v3 (Gen 2)
	v1 := CreateDummyVertex(0, 0, []types.Hash{}, signer)
	v2 := CreateDummyVertex(0, 1, []types.Hash{v1.Hash}, signer)
	v3 := CreateDummyVertex(0, 2, []types.Hash{v2.Hash}, signer)

	// 2. Out-of-order injection: Child first, then Mom.
	t.Log("Injecting Child (v3)...")
	v.DAG.AddVertex(v3, 2)
	t.Log("Injecting Mom (v2)...")
	v.DAG.AddVertex(v2, 2)

	// Neither should be in the main DAG yet.
	if v.DAG.GetVertex(v3.Hash) != nil || v.DAG.GetVertex(v2.Hash) != nil {
		t.Fatal("❌ Vertices inserted prematurely without causal history!")
	}

	// 3. Inject Grandma (v1) to trigger the recursive resolution.
	t.Log("Injecting Grandma (v1) - Starting domino effect...")
	v.DAG.AddVertex(v1, 2)

	// 4. Final Check: All three generations must be present.
	if v.DAG.GetVertex(v3.Hash) != nil && v.DAG.GetVertex(v2.Hash) != nil {
		t.Log("✅ Success! 3-generation chain resolved recursively.")
	} else {
		t.Errorf("❌ Causal domino stopped prematurely.")
	}
}

/// TestMeshOrphanResolution tests a 'Diamond' dependency structure.
/// V1 (Root) -> {V2, V3} -> V4 (Leaf).
func TestMeshOrphanResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// 1. Diamond Mesh Design
	//      V1 (Root)
	//     /  \
	//    V2   V3
	//     \  /
	//      V4 (Leaf)
	v1 := CreateDummyVertex(0, 0, []types.Hash{}, signer)
	v2 := CreateDummyVertex(0, 1, []types.Hash{v1.Hash}, signer)
	v3 := CreateDummyVertex(0, 2, []types.Hash{v1.Hash}, signer)
	v4 := CreateDummyVertex(0, 3, []types.Hash{v2.Hash, v3.Hash}, signer)

	// 2. Reverse order injection (Leaf to Root)
	t.Log("Injecting V4 (Leaf)...")
	v.DAG.AddVertex(v4, 3)
	v.DAG.AddVertex(v2, 3)
	v.DAG.AddVertex(v3, 3)

	if v.DAG.GetVertex(v4.Hash) != nil || v.DAG.GetVertex(v1.Hash) != nil {
		t.Fatal("❌ Premature insertion detected.")
	}

	// 3. Inject V1 (Root) - the final key to the mesh.
	t.Log("Injecting V1 (Root)...")
	v.DAG.AddVertex(v1, 3)

	// 4. Final verification of all participants.
	vertices := []*types.Vertex{v1, v2, v3, v4}
	for _, vtx := range vertices {
		if v.DAG.GetVertex(vtx.Hash) == nil {
			t.Errorf("❌ Vertex %s missing from final DAG!", vtx.Hash.String()[:8])
		}
	}

	if !t.Failed() {
		t.Log("✅ Success! Complex diamond mesh fully recovered.")
	}
}

/// MockFetcher simulates the network synchronization component to verify
/// if the DAG correctly triggers a sync request when a round gap is detected.
type MockFetcher struct {
	Called bool
	Hashes []types.Hash
}

func (m *MockFetcher) StartSync(missingHashes []types.Hash, suspectID int) {
	m.Called = true
	m.Hashes = missingHashes
}

/// TestSyncTriggerOnRoundGap verifies that the Fetcher is only woken up when
/// the incoming vertex's round significantly exceeds the local current round.
func TestSyncTriggerOnRoundGap(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DAG.SyncTriggerThreshold = 2

	signer, _ := NewEd25519Signer()
	mock := &MockFetcher{}

	v := NewValidator(0, cfg, signer)
	v.DAG.Fetcher = mock

	t.Run("Case 1: Small Round Gap (Sync should NOT trigger)", func(t *testing.T) {
		mock.Called = false
		currentRound := 5
		// Gap = 1, which is below threshold 2.
		vtx := CreateDummyVertex(1, 4, []types.Hash{{0x1}}, signer)
		v.DAG.AddVertex(vtx, currentRound)

		if mock.Called {
			t.Errorf("❌ Fetcher triggered for a small gap of 1! Resource waste.")
		}
	})

	t.Run("Case 2: Large Round Gap (Sync SHOULD trigger)", func(t *testing.T) {
		mock.Called = false
		currentRound := 10
		// Gap = 5 (15-10), which exceeds threshold 2.
		// Receiving a vertex from much later rounds indicates we are lagging behind.
		vtx := CreateDummyVertex(1, 15, []types.Hash{{0x2}}, signer)
		v.DAG.AddVertex(vtx, currentRound)

		if !mock.Called {
			t.Errorf("❌ Fetcher remained silent despite a gap of 5! Sync is urgent.")
		} else {
			t.Logf("✅ Success: Sync request dispatched for hash: %v", mock.Hashes)
		}
	})
}

/// TestDuplicateVertexIgnored ensures that adding the same vertex twice
/// does not affect the DAG state or size.
func TestDuplicateVertexIgnored(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	vtx := CreateDummyVertex(1, 1, []types.Hash{}, signer)

	// 1. Initial Insertion
	v.DAG.AddVertex(vtx, 1)
	initialSize := v.DAG.Size()

	// 2. Duplicate Insertion
	v.DAG.AddVertex(vtx, 1)

	// Verification: Size must remain constant.
	if v.DAG.Size() != initialSize {
		t.Errorf("❌ DAG size increased due to duplicate! (Expected: %d, Actual: %d)", initialSize, v.DAG.Size())
	} else {
		t.Log("✅ Success: Duplicate vertex was correctly ignored.")
	}
}

/// TestInvalidHashRejected verifies the integrity check.
/// A vertex with tampered data but an old hash must be rejected.
func TestInvalidHashRejected(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	vtx := CreateDummyVertex(1, 1, []types.Hash{}, signer)

	// Attack: Modify the payload/metadata without re-calculating the Hash.
	vtx.Round = 999

	v.DAG.AddVertex(vtx, 1)

	// Verification: The vertex must not exist in either the DAG or the Orphan Buffer.
	if v.DAG.GetVertex(vtx.Hash) != nil || v.DAG.Buffer.GetOrphan(vtx.Hash) != nil {
		t.Error("❌ Tampered vertex bypassed integrity checks! Security breach.")
	} else {
		t.Log("✅ Success: Forged vertex rejected due to hash mismatch.")
	}
}

/// TestPartialDependencyResolution verifies that a child vertex is only
/// released from the buffer once ALL of its parents are present in the DAG.
func TestPartialDependencyResolution(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	v1 := CreateDummyVertex(0, 1, []types.Hash{}, signer)                 // Parent 1
	v2 := CreateDummyVertex(1, 1, []types.Hash{}, signer)                 // Parent 2
	v3 := CreateDummyVertex(0, 2, []types.Hash{v1.Hash, v2.Hash}, signer) // Child

	// 1. Inject Child (v3) -> Becomes an orphan.
	v.DAG.AddVertex(v3, 2)

	// 2. Inject Parent 1 (v1) only.
	v.DAG.AddVertex(v1, 2)

	// Verification: v3 must still be an orphan because v2 is still missing.
	if v.DAG.GetVertex(v3.Hash) != nil {
		t.Fatal("❌ v3 joined the DAG prematurely while missing one parent!")
	}
	t.Log("✅ v3 is correctly waiting for the remaining dependency (v2).")

	// 3. Inject Parent 2 (v2).
	v.DAG.AddVertex(v2, 2)

	// 4. Final Verification: All dependencies met, child should be inserted.
	if v.DAG.GetVertex(v3.Hash) != nil {
		t.Log("✅ Success: v3 resolved and inserted after all parents arrived.")
	} else {
		t.Error("❌ v3 is still stuck despite all parents being present.")
	}
}

/// TestDAGConsistencyAndReconstruction ensures that two nodes can arrive at the
/// exact same DAG state regardless of the order in which vertices are received.
func TestDAGConsistencyAndReconstruction(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()

	// 1. Node A: Construct 10 vertices with complex causal dependencies.
	nodeA := NewValidator(0, cfg, signer)
	vertices := make([]*types.Vertex, 10)

	// Genesis-level vertices (Round 1)
	for i := 0; i < 3; i++ {
		vertices[i] = CreateDummyVertex(i, 1, []types.Hash{}, signer)
		nodeA.DAG.AddVertex(vertices[i], 1)
	}

	// Intertwined vertices (Rounds 2~5)
	vertices[3] = CreateDummyVertex(0, 2, []types.Hash{vertices[0].Hash, vertices[1].Hash}, signer)
	vertices[4] = CreateDummyVertex(1, 2, []types.Hash{vertices[1].Hash, vertices[2].Hash}, signer)
	vertices[5] = CreateDummyVertex(2, 3, []types.Hash{vertices[3].Hash, vertices[4].Hash}, signer)
	vertices[6] = CreateDummyVertex(0, 3, []types.Hash{vertices[0].Hash, vertices[5].Hash}, signer)
	vertices[7] = CreateDummyVertex(1, 4, []types.Hash{vertices[6].Hash, vertices[2].Hash}, signer)
	vertices[8] = CreateDummyVertex(2, 4, []types.Hash{vertices[5].Hash, vertices[7].Hash}, signer)
	vertices[9] = CreateDummyVertex(0, 5, []types.Hash{vertices[8].Hash}, signer)

	for i := 3; i < 10; i++ {
		nodeA.DAG.AddVertex(vertices[i], 5)
	}

	// 2. Node B: Initialized with a fresh state.
	nodeB := NewValidator(1, cfg, signer)

	// 3. Inject vertices into Node B in reverse order (Child to Ancestor).
	for i := 9; i >= 0; i-- {
		nodeB.DAG.AddVertex(vertices[i], 5)
	}

	// 4. Final Validation: Size and structure must match.
	if nodeA.DAG.Size() != nodeB.DAG.Size() {
		t.Errorf("❌ Convergence Failure! NodeA size: %d, NodeB size: %d",
			nodeA.DAG.Size(), nodeB.DAG.Size())
	}

	if !compareDAG(nodeA.DAG, nodeB.DAG) {
		t.Fatal("❌ Structural Mismatch: DAGs are not identical despite same inputs.")
	}

	t.Logf("✅ Success: Both nodes converged to the same state (Size: %d).", nodeB.DAG.Size())
}

/// TestAbnormalEdgeCases covers logical loops and resource exhaustion attacks.
func TestAbnormalEdgeCases(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DAG.OrphanCapacity = 2 // Extremely low capacity for testing.
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	t.Run("Möbius Loop Attack", func(t *testing.T) {
		// v1 depends on v2, and v2 depends on v1. A causal impossibility.
		h1 := types.Hash{0xaa}
		h2 := types.Hash{0xbb}

		v1 := CreateDummyVertex(0, 10, []types.Hash{h2}, signer)
		v1.Hash = h1 // Forced hash for simulation

		v2 := CreateDummyVertex(0, 11, []types.Hash{h1}, signer)
		v2.Hash = h2

		v.DAG.AddVertex(v1, 10)
		v.DAG.AddVertex(v2, 10)

		// Result: Both should remain in the Orphan Buffer without crashing the system.
		if v.DAG.Size() != 0 {
			t.Error("❌ Circular dependency should not be inserted into the DAG.")
		}
	})

	t.Run("Orphanage Capacity Limit", func(t *testing.T) {
		// 1. Fill buffer to capacity.
		for i := 0; i < 2; i++ {
			vtx := CreateDummyVertex(0, i+1, []types.Hash{{byte(i + 100)}}, signer)
			v.DAG.AddVertex(vtx, 1)
		}

		// 2. Inject extra orphan.
		vtxExtra := CreateDummyVertex(0, 99, []types.Hash{{0xff}}, signer)
		v.DAG.AddVertex(vtxExtra, 1)

		size := v.DAG.Buffer.Size()
		if size <= 2 {
			t.Logf("✅ Success: Buffer eviction policy enforced. Size: %d", size)
		} else {
			t.Errorf("❌ Buffer overflow! Current size: %d", size)
		}
	})
}

/// TestRandomizedDeliveryConvergence tests thread-safety and idempotency under high concurrency.
// Thread-Safety, Order-Independence, Idempotency
func TestRandomizedDeliveryConvergence(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()

	nodeA := NewValidator(0, cfg, signer)
	nodeB := NewValidator(1, cfg, signer)

	vertices := generateComplexDAG(50)

	// A: Sequential delivery.
	for _, vtx := range vertices {
		nodeA.DAG.AddVertex(vtx, 10)
	}

	// B: Parallel delivery with random delays and duplicates.
	var wg sync.WaitGroup
	shuffled := shuffle(vertices)

	for _, vtx := range shuffled {
		wg.Add(1)
		go func(v *types.Vertex) {
			defer wg.Done()

			// Simulate network jitter.
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)

			nodeB.DAG.AddVertex(v, 10)

			// Simulate re-transmission / duplication.
			if rand.Intn(2) == 0 {
				nodeB.DAG.AddVertex(v, 10)
			}
		}(vtx)
	}

	wg.Wait()

	if !compareDAGFull(nodeA.DAG, nodeB.DAG) {
		t.Fatal("❌ Concurrency Failure: DAGs diverged under parallel injection.")
	}
	t.Log("✅ Success: Deterministic convergence achieved under race conditions.")
}

/// TestByzantineResilience validates that the Slasher catches malformed vertices.
func TestByzantineResilience(t *testing.T) {
	cfg := config.DefaultConfig()
	signer, _ := NewEd25519Signer()
	v := NewValidator(0, cfg, signer)

	// Case A: Malformed parent sorting violation.
	v1 := CreateDummyVertex(1, 1, []types.Hash{}, signer)
	v2 := CreateDummyVertex(1, 1, []types.Hash{}, signer)

	// Intentionally unsorted parents.
	malformed := []types.Hash{v2.Hash, v1.Hash}
	if v1.Hash.String() < v2.Hash.String() {
		malformed = []types.Hash{v2.Hash, v1.Hash}
	} else {
		malformed = []types.Hash{v1.Hash, v2.Hash}
	}

	vByz := CreateByzantineVertex(1, 1, malformed, signer)
	v.DAG.AddVertex(vByz, 1)

	// Check if Slasher applied penalty for MsgInvalidPayload.
	if v.Slasher.GetPenalty(1) == 0 {
		t.Fatal("❌ Failed to detect malformed parent sorting (Byzantine).")
	}

	// Case B: Memory Attack (Ghost orphans)
	for i := 0; i < cfg.DAG.OrphanCapacity+5; i++ {
		ghost := CreateDummyVertex(2, 5, []types.Hash{{byte(i + 50)}}, signer)
		v.DAG.AddVertex(ghost, 5)
	}

	if v.DAG.Buffer.Size() > cfg.DAG.OrphanCapacity {
		t.Errorf("❌ Orphan Buffer exceeded capacity limit: %d", v.DAG.Buffer.Size())
	}
}
