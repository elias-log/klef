// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

/*
Hash provides a fixed-size, value-type identifier for consensus primitives.

Key properties:
- Comparable: As a [32]byte array, it can be used directly as a map key without casting.
- Value Semantics: Passed by value, ensuring immutability across function boundaries.
- Deterministic Ordering: Supports lexicographical comparison via bytes.Compare.
*/

package types

import (
	"bytes"
	"encoding/hex"
	"sort"
)

/// Hash represents a fixed-size 32-byte binary identifier (SHA-256).
type Hash [32]byte

/// String returns the hex-encoded string representation of the hash.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

/// Bytes returns the underlying byte slice of the hash.
// This allows the Hash (array) to be used where a []byte (slice) is required
// without the user needing to perform manual slicing.
func (h Hash) Bytes() []byte {
	return h[:]
}

/// Equal determines if two hashes are byte-for-byte identical.
func (h Hash) Equal(other Hash) bool {
	return bytes.Equal(h[:], other[:])
}

/// Compare performs a lexicographical comparison.
// Returns:
// -1 if h < other
//  0 if h == other
// +1 if h > other
func (h Hash) Compare(other Hash) int {
	return bytes.Compare(h[:], other[:])
}

func SortHashes(hashes []Hash) {
	sort.Sort(HashSlice(hashes))
}

/*
HashSlice implements sort.Interface for a slice of Hashes.

This allows for deterministic ordering of parent lists or transaction sets
using the standard sort package: sort.Sort(HashSlice(myHashes)).
*/
type HashSlice []Hash

func (s HashSlice) Len() int           { return len(s) }
func (s HashSlice) Less(i, j int) bool { return s[i].Compare(s[j]) < 0 }
func (s HashSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
