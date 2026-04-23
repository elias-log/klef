// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 elias-log

package types

type MessageType uint8

const (
	MsgVertex         MessageType = iota // 0: New DAG vertex proposal
	MsgVote                              // 1: Consensus vote for a specific round
	MsgFetchReq                          // 2: Request for missing causal data
	MsgFetchRes                          // 3: Response containing requested vertices
	MsgEvidence                          // 4: Proof of protocol violation (e.g., Equivocation)
	MsgInvalidPayload                    // 5: Proof of malformed data (e.g., Duplicate parents)
)

func (t MessageType) String() string {
	switch t {
	case MsgVertex:
		return "VERTEX"
	case MsgVote:
		return "VOTE"
	case MsgFetchReq:
		return "FETCH_REQ"
	case MsgFetchRes:
		return "FETCH_RES"
	case MsgEvidence:
		return "EVIDENCE"
	case MsgInvalidPayload:
		return "INVALID_PAYLOAD"
	default:
		return "UNKNOWN"
	}
}

/// Message represents the unified envelope for all P2P communication.
type Message struct {
	FromID       int
	CurrentRound int
	Type         MessageType
	Signature    Signature

	// Payloads (Mutual Exclusivity handled by Type field)
	Vote     *Vote
	Vertex   *Vertex
	FetchReq *FetchRequest
	FetchRes *FetchResponse
	Evidence *Evidence
}

/// FetchRequest is used to pull missing causal dependencies from peers.
type FetchRequest struct {
	MissingHashes []Hash
}

/// FetchResponse acts as a container for bulk vertex retrieval.
type FetchResponse struct {
	Vertices []*Vertex
}

/*
  [Evidence Handling Rules]
  1. Integrity: All proofs must contain valid signatures from the accused Author.
  2. Optimization: Future iterations should use (Hash + Signature) instead of full
     Vertex objects to minimize bandwidth.
  3. Propagation: Validated evidence must be broadcasted to all neighbors via Gossip.
*/

/// Evidence represents a cryptographic proof of a protocol violation.
type Evidence struct {
	TargetID    int         // ID of the validator being accused
	Type        MessageType // Nature of the violation (e.g., Equivocation or Malformed Payload)
	Proof1      *Vertex     // Primary piece of evidence
	Proof2      *Vertex     // Secondary piece of evidence (required for Equivocation)
	ReporterID  int         // ID of the validator who first detected and reported the violation
	Timestamp   int64       // Reporting time (UNIX epoch)
	Description string      // Detailed reasoning for the accusation
}

/// IsValid performs a self-contained logical validation of the evidence.
func (e *Evidence) IsValid() bool {
	// Basic verification: Evidence must have at least one proof and match the target ID.
	if e.Proof1 == nil || e.Proof1.Author != e.TargetID {
		return false
	}

	// Case 1: Equivocation (Double Voting/Proposing)
	// Requires two different vertices signed by the same author in the same round.
	if e.Proof2 != nil {
		if e.Proof1.Author != e.Proof2.Author {
			return false
		}
		if e.Proof1.Round == e.Proof2.Round && e.Proof1.Hash != e.Proof2.Hash {
			return true
		}
	}

	// Case 2: Payload Violation (e.g., Duplicate or Malformed Parents)
	// Can be proven using a single vertex payload.
	if e.Type == MsgInvalidPayload {
		isMalformed, _ := CheckMalformed(e.Proof1.Parents)
		return isMalformed
	}

	return false
}
