/*
   [Arachnet-BFT Vertex Specification]

   이 파일은 DAG의 atomic한 'Vertex'와 그 정당성을 보장하는 'QC'를 정의하네.

   1. Vertex:
      - 생성자의 서명과 부모들에 대한 참조를 담고 있는 데이터 블록일세.
      - 단순히 부모 해시만 나열하는 것이 아니라, 부모들이 정족수(2f+1)를 채웠다는
        증거(ParentQC)를 반드시 포함해야 하네.

   2. QC (Quorum Certificate):
      - 특정 라운드, 특정 해시에 대해 Validator 집단이 승인했다는 '보증서'일세.
      - 이 QC가 존재함으로써, 새로운 노드는 과거의 모든 트랜잭션을 일일이
        검증하지 않아도 이 지점이 '합의된 과거'임을 신뢰할 수 있네.

   3. Deterministic Hashing (결정론적 해싱):
      - 네트워크상의 모든 노드는 동일한 Vertex에 대해 1비트의 오차도 없이
        동일한 해시값을 도출해야 하네. 이를 위해 BigEndian 통일, 길이 접두사
        사용, 그리고 가변 데이터(Map)의 정렬 처리를 엄격히 준수하네.
*/

package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
)

// Vertex는 DAG의 기본 단위라네.
// 대문자 V로 시작해야 밖에서 보이네!
type Vertex struct {
	Author    int      // 생성자 ID
	Round     int      // 라운드 번호
	Parents   []string // 부모 Vertex들의 해시 리스트
	ParentQCs []*QC    // 부모들에 대한 Quorum Certificate
	Timestamp int64    // vertex 생성 시점
	Payload   [][]byte // 포함된 트랜잭션들
	Signature []byte   // 생성자의 서명
	Hash      string   // 이 Vertex의 고유 식별자 (일단 SHA-256)
}

// 일단 QCGlobal을 디폴트로 쓰지만, 계층구조가 구현된 이후를 고려한 것이네.
type QCType int

const (
	QCGlobal    QCType = iota // DAG 기반 2f+1
	QCCommittee               // 커미티 내부 3/4 (Jolteon 등)
)

// QC는 특정 라운드의 Vertex에 대해 2f+1개의 서명이 모였다는 증명일세.
type QC struct {
	Type       QCType
	VertexHash string
	Round      int
	ProposerID int            // 이 QC를 조립하고 제안한 주동자 ID
	Signatures map[int][]byte // 동조자들
	//TODO: map을 bitmask로 바꿔서 overhead 감소, BLS Aggregate Signature
}

func (v *Vertex) CalculateHash() string {
	// CalculateHash: Deterministic Binary
	// [Deterministic Binary Serialization Rules for Hashing]
	// 이 메서드는 지구상 어떤 노드에서도 동일한 Vertex에 대해 1비트의 오차 없이
	// 동일한 바이트 스트림을 생성해야 하네. 이를 위해 아래의 '엄격한 규칙'을 준수하네:
	//
	// 1. 고정 엔디언 (Fixed Endianness):
	//    시스템 아키텍처(Little/Big Endian)에 상관없이 'BigEndian'으로 바이트 순서를 통일함.
	//
	// 2. 고정 순서 (Canonical Ordering):
	//    필드 선언 순서가 아닌, 본 로직에서 정의한 명시적 순서대로 버퍼에 기록함.
	//    (Author -> Round -> Parents Count -> Parents Hashes -> Payload Count -> Payloads)
	//
	// 3. 길이 접두사 (Length Prefixing):
	//    문자열, 슬라이스 등 가변 길이 데이터는 반드시 그 길이를 먼저 기록(int32)하여
	//    데이터 간의 경계를 명확히 하고 모호성을 제거함.
	//
	// 4. 무상태성 (Stateless):
	//    외부 환경이나 메타데이터(예: gob의 타입 정보)를 배제하고 오직 순수 데이터만 추출함

	buf := new(bytes.Buffer)

	// 1. Author & Round
	binary.Write(buf, binary.BigEndian, int32(v.Author))
	binary.Write(buf, binary.BigEndian, int64(v.Round))

	// 2. Parents ([Count(4)] + [Hash Strings...])
	binary.Write(buf, binary.BigEndian, int32(len(v.Parents)))
	for _, pHash := range v.Parents {
		// 문자열 길이를 먼저 쓰고 본문을 쓰는 것이 더 완벽하지만,
		// 해시값이 고정 길이라면 바로 써도 무방하네.
		buf.WriteString(pHash) // 해시 문자열을 그대로 버퍼에 흘려넣네.
	}

	// 3. ParentQC ([Round(8)] + [VertexHash(len prefix + data)] + [Signatures Count(4)] + [Signatures...])
	binary.Write(buf, binary.BigEndian, int32(len(v.ParentQCs)))
	for _, qc := range v.ParentQCs {
		if qc == nil {
			continue
		}

		binary.Write(buf, binary.BigEndian, int8(qc.Type))
		binary.Write(buf, binary.BigEndian, int64(qc.Round))
		binary.Write(buf, binary.BigEndian, int32(qc.ProposerID))

		// VertexHash 기록
		binary.Write(buf, binary.BigEndian, int32(len(qc.VertexHash)))
		buf.WriteString(qc.VertexHash)

		// Signatures 정렬 및 기록
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

	// 4. Timestamp
	binary.Write(buf, binary.BigEndian, v.Timestamp)

	// 5. Payload ([Count(4)] + [Tx Length(4) + Tx Data...])
	binary.Write(buf, binary.BigEndian, int32(len(v.Payload)))
	for _, tx := range v.Payload {
		binary.Write(buf, binary.BigEndian, int32(len(tx)))
		buf.Write(tx)
	}

	// 이제 이 buf.Bytes()는 지구상 어디서든 똑같은 Vertex에 대해 똑같은 바이트를 뱉네!
	hash := sha256.Sum256(buf.Bytes())
	return hex.EncodeToString(hash[:])
}

func (v *Vertex) Normalize() (hadDuplicate bool) {
	beforeLen := len(v.Parents)

	// dedup + sort
	v.Parents = deduplicateAndSort(v.Parents)

	// 단순 길이 비교는 순서 바뀜을 체크 못 하므로,
	// deep equal이나 hash 비교를 쓰기도 하지만 지금은 중복 제거 위주로 하겠네.
	return len(v.Parents) != beforeLen
}

// deduplicateAndSort: 중복 제거 후 정렬된 슬라이스를 반환하는 도우미 함수일세.
func deduplicateAndSort(in []string) []string {
	if len(in) <= 1 {
		sort.Strings(in) // 1개일 때도 정렬은 수행하네.
		return in
	}

	seen := make(map[string]struct{})
	unique := make([]string, 0, len(in))
	for _, s := range in {
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}
			unique = append(unique, s)
		}
	}
	sort.Strings(unique)
	return unique
}

// Canonical Parent Ordering is part of protocol validity.
// Any deviation is treated as Byzantine behavior.
func CheckMalformed(parents []string) (bool, string) {
	if len(parents) <= 1 {
		return false, ""
	}

	for i := 0; i < len(parents)-1; i++ {
		// 1. 중복 체크
		if parents[i] == parents[i+1] {
			return true, fmt.Sprintf("Duplicate parent hash detected: %s", parents[i])
		}
		// 2. 정렬 체크 (사전순 위반)
		if parents[i] > parents[i+1] {
			return true, fmt.Sprintf("Unsorted parents: %s comes before %s", parents[i], parents[i+1])
		}
	}
	return false, ""
}

// HasDuplicate: 슬라이스 내에 중복된 해시가 있는지 검사하네. (검증용)
func HasDuplicate(elements []string) bool {
	encountered := make(map[string]struct{})
	for _, v := range elements {
		if _, ok := encountered[v]; ok {
			return true
		}
		encountered[v] = struct{}{}
	}
	return false
}
