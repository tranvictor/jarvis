package ledgerbtc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"sort"
)

type hash32 = [32]byte

var nilHash hash32

func sha(b ...[]byte) hash32 {
	h := sha256.New()
	for _, p := range b {
		h.Write(p)
	}
	var out hash32
	copy(out[:], h.Sum(nil))
	return out
}

// elementHash is the Merkle leaf of a preimage.
func elementHash(preimage []byte) hash32 { return sha([]byte{0x00}, preimage) }

func combine(l, r hash32) hash32 { return sha([]byte{0x01}, l[:], r[:]) }

// largestPow2Below returns the largest power of two strictly below n (n >= 2).
func largestPow2Below(n int) int {
	p := 1
	for p*2 < n {
		p *= 2
	}
	return p
}

// tree is the Merkle tree the Ledger Bitcoin app expects: left-complete, so
// every left subtree holds the largest power of two of leaves below the
// size of its parent.
type tree struct{ leaves []hash32 }

func newTree(leaves []hash32) *tree { return &tree{leaves: leaves} }

func rootOf(leaves []hash32) hash32 {
	switch len(leaves) {
	case 0:
		return nilHash
	case 1:
		return leaves[0]
	}
	k := largestPow2Below(len(leaves))
	return combine(rootOf(leaves[:k]), rootOf(leaves[k:]))
}

func (t *tree) root() hash32 { return rootOf(t.leaves) }

// proof returns the sibling hashes from the leaf up to the root.
func (t *tree) proof(index int) []hash32 { return proofOf(t.leaves, index) }

func proofOf(leaves []hash32, index int) []hash32 {
	if len(leaves) <= 1 {
		return nil
	}
	k := largestPow2Below(len(leaves))
	if index < k {
		return append(proofOf(leaves[:k], index), rootOf(leaves[k:]))
	}
	return append(proofOf(leaves[k:], index-k), rootOf(leaves[:k]))
}

func (t *tree) indexOf(h hash32) (int, bool) {
	for i, l := range t.leaves {
		if l == h {
			return i, true
		}
	}
	return 0, false
}

func writeVarint(n uint64) []byte {
	switch {
	case n <= 0xfc:
		return []byte{byte(n)}
	case n <= 0xffff:
		b := []byte{0xfd, 0, 0}
		binary.LittleEndian.PutUint16(b[1:], uint16(n))
		return b
	case n <= 0xffffffff:
		b := []byte{0xfe, 0, 0, 0, 0}
		binary.LittleEndian.PutUint32(b[1:], uint32(n))
		return b
	}
	b := make([]byte, 9)
	b[0] = 0xff
	binary.LittleEndian.PutUint64(b[1:], n)
	return b
}

func readVarint(r *bytes.Reader) (uint64, error) {
	first, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	var n int
	switch first {
	case 0xfd:
		n = 2
	case 0xfe:
		n = 4
	case 0xff:
		n = 8
	default:
		return uint64(first), nil
	}
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r, buf[:n]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

// kv is one entry of a PSBT map.
type kv struct{ key, value []byte }

// Map is a PSBT key-value map. Keys are the raw key bytes (type, then key
// data), exactly what sits between the length prefix and the value in a
// serialized PSBT.
type Map []kv

// Add appends an entry. key is the PSBT key type followed by any key data.
func (m *Map) Add(key, value []byte) { *m = append(*m, kv{key: key, value: value}) }

func (m Map) sorted() Map {
	out := append(Map(nil), m...)
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i].key, out[j].key) < 0 })
	return out
}

// commitment is count || root(keys) || root(values), the Merkleized form the
// device receives instead of the map.
func (m Map) commitment() []byte {
	s := m.sorted()
	keys := make([]hash32, len(s))
	vals := make([]hash32, len(s))
	for i, e := range s {
		keys[i] = elementHash(e.key)
		vals[i] = elementHash(e.value)
	}
	kr, vr := rootOf(keys), rootOf(vals)
	out := writeVarint(uint64(len(s)))
	out = append(out, kr[:]...)
	return append(out, vr[:]...)
}
