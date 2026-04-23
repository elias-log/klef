// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Vertex defines the atomic unit of the Klef DAG and its Quorum Certificates.

Key properties:
- Causal Linking: Each vertex references its parents and includes QCs
  to justify their validity.
- Deterministic Hashing: Ensures identical byte-level representation across nodes
  via strict BigEndian serialization and canonical field ordering.
- Byzantine Fault Tolerance: Malformed structures (unsorted or duplicate parents)
  are detected during validation and may trigger penalties.

Note:
- Currently uses hex-encoded strings for hashes.
- Future optimization: Transition to fixed-size [32]byte for reduced memory footprint.
*/

package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
)

// QCType defines the scope of a Quorum Certificate.
type QCType int

const (
	QCGlobal    QCType = iota // Standard 2f+1 DAG consensus
	QCCommittee               // High-speed 3/4 committee consensus (e.g., Jolteon)
)

/// QC (Quorum Certificate) is a cryptographic proof that a threshold of validators
/// has acknowledged a specific Vertex at a given Round.
type QC struct {
	Type       QCType
	VertexHash Hash
	Round      int
	ProposerID int               // Node ID that aggregated this QC
	Signatures map[int]Signature // Mapping of NodeID to its ECDSA/Ed25519 signature
}

/// Vertex is the fundamental block of the DAG, containing transactions and proofs.
type Vertex struct {
	Author    int       // ID of the validator who created this vertex
	Round     int       // Logical clock/height in the DAG
	Parents   []Hash    // List of parent vertex hashes (hex encoded)
	ParentQCs []*QC     // Quorum Certificates proving the validity of parents
	Timestamp int64     // Unix timestamp of creation
	Payload   [][]byte  // Batch of transactions in serialized form
	Signature Signature // Author's cryptographic signature over the vertex hash
	Hash      Hash      // Unique identifier derived via CalculateHash()
}

/// CalculateHash generates a globally consistent SHA-256 hash of the vertex.
/// It strictly adheres to BigEndian serialization and canonical ordering.
func (v *Vertex) CalculateHash() Hash {
	buf := new(bytes.Buffer)

	// 1. Author (int32) & Round (int64)
	binary.Write(buf, binary.BigEndian, int32(v.Author))
	binary.Write(buf, binary.BigEndian, int64(v.Round))

	// 2. Parents: [Count(int32)] + [Hash Strings]
	binary.Write(buf, binary.BigEndian, int32(len(v.Parents)))
	for _, pHash := range v.Parents {
		// Note: Include hash length for future flexibility.
		binary.Write(buf, binary.BigEndian, int32(len(pHash)))
		buf.Write(pHash.Bytes())
	}

	// 3. ParentQCs: [Count(int32)] + [QC Encodings]
	binary.Write(buf, binary.BigEndian, int32(len(v.ParentQCs)))
	for _, qc := range v.ParentQCs {
		if qc == nil {
			continue
		}

		binary.Write(buf, binary.BigEndian, int8(qc.Type))
		binary.Write(buf, binary.BigEndian, int64(qc.Round))
		binary.Write(buf, binary.BigEndian, int32(qc.ProposerID))

		// VertexHash with length prefix to prevent ambiguity
		binary.Write(buf, binary.BigEndian, int32(len(qc.VertexHash)))
		buf.Write(qc.VertexHash.Bytes())

		// Signatures: Sorted by NodeID for determinism
		ids := make([]int, 0, len(qc.Signatures))
		for id := range qc.Signatures {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		binary.Write(buf, binary.BigEndian, int32(len(qc.Signatures)))
		for _, id := range ids {
			sig := qc.Signatures[id]
			binary.Write(buf, binary.BigEndian, int32(id))
			binary.Write(buf, binary.BigEndian, int32(len(sig)))
			buf.Write(sig)
		}
	}

	// 4. Timestamp (int64)
	binary.Write(buf, binary.BigEndian, v.Timestamp)

	// 5. Payload: [Count(int32)] + [Tx Length(int32) + Tx Data]
	binary.Write(buf, binary.BigEndian, int32(len(v.Payload)))
	for _, tx := range v.Payload {
		binary.Write(buf, binary.BigEndian, int32(len(tx)))
		buf.Write(tx)
	}

	sum := sha256.Sum256(buf.Bytes())
	return Hash(sum)
}

/// Normalize sanitizes the vertex structure by removing duplicate parents
/// and ensuring canonical string ordering.
///
/// Note: This does not validate protocol correctness; use CheckMalformed for strict validation.
/// Returns true if any duplicates were detected and removed, signaling
/// potential inefficiency or malformed input from the creator.
func (v *Vertex) Normalize() (hadDuplicate bool) {
	beforeLen := len(v.Parents)

	// In-place deduplication and sorting to satisfy canonical hashing rules.
	v.Parents = deduplicateAndSort(v.Parents)

	// If length changed, duplicates were present.
	// NOTE: This signals a mutation but does not inherently validate
	// the original input's canonical ordering—it simply enforces it post-hoc.
	return len(v.Parents) != beforeLen
}

/// deduplicateAndSort is an internal helper that returns a unique,
/// lexicographically sorted slice of strings.
// This is essential for maintaining a deterministic byte stream during hashing.
func deduplicateAndSort(in []Hash) []Hash {
	// 1. Edge case: Nothing to sort or deduplicate.
	if len(in) <= 1 {
		return in
	}

	// 2. Deduplication using a map.
	seen := make(map[Hash]struct{})
	unique := make([]Hash, 0, len(in))
	for _, h := range in {
		if _, exists := seen[h]; !exists {
			seen[h] = struct{}{}
			unique = append(unique, h)
		}
	}

	// 3. Deterministic Sorting.
	SortHashes(unique)

	return unique
}

/// CheckMalformed verifies if the parent hashes adhere to the canonical
/// protocol rules (Sorting and Uniqueness).
///
/// Formal Property:
/// For any index i, Parents[i] < Parents[i+1] must hold true (Strict Monotonicity).
func CheckMalformed(parents []Hash) (bool, string) {
	if len(parents) <= 1 {
		return false, ""
	}

	for i := 0; i < len(parents)-1; i++ {
		// 1. Duplicate Detection
		if parents[i] == parents[i+1] {
			return true, fmt.Sprintf("Duplicate parent hash detected: %s", parents[i])
		}
		// 2. Canonical Order Verification (Lexicographical)
		// Any deviation is treated as a protocol violation (Byzantine).
		// parents[i] > parents[i+1]
		if parents[i].Compare(parents[i+1]) > 0 {
			return true, fmt.Sprintf("Unsorted parents: %s comes before %s", parents[i], parents[i+1])
		}
	}
	return false, ""
}

/// HasDuplicate checks for the existence of any redundant hashes within a slice.
// Used as a lightweight guardrail in pre-validation stages.
func HasDuplicate(elements []Hash) bool {
	encountered := make(map[Hash]struct{})
	for _, v := range elements {
		if _, ok := encountered[v]; ok {
			return true
		}
		encountered[v] = struct{}{}
	}
	return false
}

func SortVertices(vtxs []*Vertex) {
	sort.Slice(vtxs, func(i, j int) bool {
		return vtxs[i].Hash.Compare(vtxs[j].Hash) < 0
	})
}
