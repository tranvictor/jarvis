package ledgerbtc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

const (
	cmdYield          = 0x10
	cmdGetPreimage    = 0x40
	cmdGetLeafProof   = 0x41
	cmdGetLeafIndex   = 0x42
	cmdGetMoreElement = 0xA0
)

// interpreter answers the device's requests for data it does not hold. The
// device never sees the PSBT; it asks for individual map entries and checks
// each against the Merkle roots it was given.
type interpreter struct {
	preimages map[hash32][]byte
	trees     map[hash32]*tree
	queue     [][]byte
	yielded   [][]byte
}

func newInterpreter() *interpreter {
	return &interpreter{preimages: map[hash32][]byte{}, trees: map[hash32]*tree{}}
}

func (in *interpreter) addPreimage(b []byte) { in.preimages[sha256.Sum256(b)] = b }

// addList makes a Merkleized list of elements answerable.
func (in *interpreter) addList(elements [][]byte) {
	leaves := make([]hash32, len(elements))
	for i, e := range elements {
		in.addPreimage(append([]byte{0x00}, e...))
		leaves[i] = elementHash(e)
	}
	t := newTree(leaves)
	in.trees[t.root()] = t
}

// addMap makes the key tree and the value tree of a map answerable.
func (in *interpreter) addMap(m Map) {
	s := m.sorted()
	keys := make([][]byte, len(s))
	vals := make([][]byte, len(s))
	for i, e := range s {
		keys[i], vals[i] = e.key, e.value
	}
	in.addList(keys)
	in.addList(vals)
}

func (in *interpreter) execute(req []byte) ([]byte, error) {
	if len(req) == 0 {
		return nil, errors.New("ledger: empty client command")
	}
	switch req[0] {
	case cmdYield:
		in.yielded = append(in.yielded, append([]byte(nil), req[1:]...))
		return nil, nil
	case cmdGetPreimage:
		return in.getPreimage(req[1:])
	case cmdGetLeafProof:
		return in.getLeafProof(req[1:])
	case cmdGetLeafIndex:
		return in.getLeafIndex(req[1:])
	case cmdGetMoreElement:
		return in.getMore(req[1:])
	}
	return nil, fmt.Errorf("ledger: unknown client command 0x%02x", req[0])
}

func (in *interpreter) getPreimage(req []byte) ([]byte, error) {
	if len(req) != 33 || req[0] != 0 {
		return nil, errors.New("ledger: malformed GET_PREIMAGE request")
	}
	var h hash32
	copy(h[:], req[1:])
	pre, ok := in.preimages[h]
	if !ok {
		return nil, fmt.Errorf("ledger asked for an unknown preimage %x", h)
	}
	lenEnc := writeVarint(uint64(len(pre)))
	max := 255 - len(lenEnc) - 1
	size := len(pre)
	if size > max {
		size = max
		for _, b := range pre[size:] {
			in.queue = append(in.queue, []byte{b})
		}
	}
	out := append([]byte(nil), lenEnc...)
	out = append(out, byte(size))
	return append(out, pre[:size]...), nil
}

func (in *interpreter) getLeafProof(req []byte) ([]byte, error) {
	r := bytes.NewReader(req)
	var root hash32
	if _, err := io.ReadFull(r, root[:]); err != nil {
		return nil, err
	}
	size, err := readVarint(r)
	if err != nil {
		return nil, err
	}
	idx, err := readVarint(r)
	if err != nil {
		return nil, err
	}
	t, ok := in.trees[root]
	if !ok {
		return nil, fmt.Errorf("ledger asked for an unknown Merkle root %x", root)
	}
	if idx >= size || uint64(len(t.leaves)) != size {
		return nil, errors.New("ledger: invalid Merkle leaf index or tree size")
	}
	if len(in.queue) != 0 {
		return nil, errors.New("ledger: proof requested while data is still queued")
	}
	proof := t.proof(int(idx))
	fit := (255 - 32 - 1 - 1) / 32
	if fit > len(proof) {
		fit = len(proof)
	}
	for _, p := range proof[fit:] {
		p := p
		in.queue = append(in.queue, p[:])
	}
	leaf := t.leaves[idx]
	out := append([]byte(nil), leaf[:]...)
	out = append(out, byte(len(proof)), byte(fit))
	for _, p := range proof[:fit] {
		out = append(out, p[:]...)
	}
	return out, nil
}

func (in *interpreter) getLeafIndex(req []byte) ([]byte, error) {
	if len(req) != 64 {
		return nil, errors.New("ledger: malformed GET_MERKLE_LEAF_INDEX request")
	}
	var root, leaf hash32
	copy(root[:], req[:32])
	copy(leaf[:], req[32:])
	t, ok := in.trees[root]
	if !ok {
		return nil, fmt.Errorf("ledger asked for an unknown Merkle root %x", root)
	}
	idx, found := t.indexOf(leaf)
	if !found {
		return append([]byte{0}, writeVarint(0)...), nil
	}
	return append([]byte{1}, writeVarint(uint64(idx))...), nil
}

func (in *interpreter) getMore(req []byte) ([]byte, error) {
	if len(req) != 0 {
		return nil, errors.New("ledger: malformed GET_MORE_ELEMENTS request")
	}
	if len(in.queue) == 0 {
		return nil, errors.New("ledger asked for more elements but none are queued")
	}
	size := len(in.queue[0])
	for _, e := range in.queue {
		if len(e) != size {
			return nil, errors.New("ledger: queued elements differ in length")
		}
	}
	var payload []byte
	n := 0
	for len(in.queue) > 0 && len(payload)+size <= 253 {
		payload = append(payload, in.queue[0]...)
		in.queue = in.queue[1:]
		n++
	}
	return append([]byte{byte(n), byte(size)}, payload...), nil
}
