// Package ledgerbtc speaks the Ledger Bitcoin application protocol (the
// "new" protocol of app-bitcoin-new, CLA 0xE1) for BIP-84 single-signature
// accounts. It implements the device's client-side commands (Merkle proofs
// and preimage queries) that drive PSBT signing.
package ledgerbtc

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

const (
	claBitcoin = 0xE1
	claDefault = 0xB0
	claFrame   = 0xF8

	insGetPubkey       = 0x00
	insGetWalletAddr   = 0x03
	insSignPSBT        = 0x04
	insMasterFP        = 0x05
	insContinue        = 0x01
	insGetVersion      = 0x01
	protocolVersion    = 1
	swOK               = 0x9000
	swInterrupted      = 0xE000
	swDenied           = 0x6985
	swWrongAppOrLocked = 0x6E00
)

// Status words that the Bitcoin app and the Ledger OS return.
var (
	ErrDenied   = errors.New("rejected on the Ledger")
	ErrWrongApp = errors.New("the Bitcoin app is not open on the Ledger (open it, or unlock the device)")
)

// StatusError is a non-success status word from the device.
type StatusError struct {
	SW  uint16
	Ins byte
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("ledger returned status 0x%04X (instruction 0x%02X)", e.SW, e.Ins)
}

func statusErr(sw uint16, ins byte) error {
	switch sw {
	case swDenied:
		return fmt.Errorf("%w (0x6985)", ErrDenied)
	case swWrongAppOrLocked, 0x6E01, 0x6D00, 0x6511, 0x5515:
		return fmt.Errorf("%w (status 0x%04X)", ErrWrongApp, sw)
	}
	return &StatusError{SW: sw, Ins: ins}
}

// Policy is a wallet policy. Only the default single-signature policy
// wpkh(@0/**) is used, which the app signs for without prior registration.
type Policy struct {
	Name     string
	Template string
	Keys     []string
}

// DefaultWPKH returns the BIP-84 policy for an account key. keyInfo is
// "[masterfingerprint/84'/coin'/account']xpub".
func DefaultWPKH(keyInfo string) Policy {
	return Policy{Template: "wpkh(@0/**)", Keys: []string{keyInfo}}
}

func (p Policy) serialize() []byte {
	keyHashes := make([]hash32, len(p.Keys))
	for i, k := range p.Keys {
		keyHashes[i] = elementHash([]byte(k))
	}
	tmplHash := sha256.Sum256([]byte(p.Template))
	root := rootOf(keyHashes)
	out := []byte{2, byte(len(p.Name))}
	out = append(out, p.Name...)
	out = append(out, writeVarint(uint64(len(p.Template)))...)
	out = append(out, tmplHash[:]...)
	out = append(out, writeVarint(uint64(len(p.Keys)))...)
	return append(out, root[:]...)
}

func (p Policy) id() hash32 { return sha256.Sum256(p.serialize()) }

type Client struct {
	T Transport
	// Debug, when set, receives every APDU for troubleshooting.
	Debug func(format string, args ...any)
}

func (c *Client) exchange(cla, ins, p1 byte, data []byte) ([]byte, uint16, error) {
	resp, sw, err := c.T.Exchange(cla, ins, p1, protocolVersion, data)
	if c.Debug != nil {
		c.Debug("=> cla=%02x ins=%02x p1=%02x %x\n<= sw=%04x %x", cla, ins, p1, data, sw, resp)
	}
	return resp, sw, err
}

// Version returns the running app's name and version, for example
// ("Bitcoin Test", "2.4.2").
func (c *Client) Version() (name, version string, err error) {
	resp, sw, err := c.T.Exchange(claDefault, insGetVersion, 0, 0, nil)
	if err != nil {
		return "", "", err
	}
	if sw != swOK {
		return "", "", statusErr(sw, insGetVersion)
	}
	r := bytes.NewReader(resp)
	if f, _ := r.ReadByte(); f != 1 {
		return "", "", errors.New("ledger: unexpected GET_VERSION format")
	}
	readStr := func() (string, error) {
		n, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		return string(b), nil
	}
	if name, err = readStr(); err != nil {
		return "", "", err
	}
	if version, err = readStr(); err != nil {
		return "", "", err
	}
	return name, version, nil
}

// run sends the first APDU and then serves the device's client commands
// until it answers with a final status.
func (c *Client) run(ins byte, data []byte, in *interpreter) ([]byte, error) {
	resp, sw, err := c.exchange(claBitcoin, ins, 0, data)
	for err == nil && sw == swInterrupted {
		if in == nil {
			return nil, errors.New("ledger: unexpected request for host data")
		}
		var out []byte
		if out, err = in.execute(resp); err != nil {
			return nil, err
		}
		resp, sw, err = c.exchange(claFrame, insContinue, 0, out)
	}
	if err != nil {
		return nil, err
	}
	if sw != swOK {
		return nil, statusErr(sw, ins)
	}
	return resp, nil
}

func (c *Client) MasterFingerprint() ([4]byte, error) {
	resp, err := c.run(insMasterFP, nil, nil)
	if err != nil {
		return [4]byte{}, err
	}
	if len(resp) != 4 {
		return [4]byte{}, fmt.Errorf("ledger: master fingerprint has %d bytes", len(resp))
	}
	var fp [4]byte
	copy(fp[:], resp)
	return fp, nil
}

func pathBytes(path []uint32) []byte {
	out := []byte{byte(len(path))}
	for _, p := range path {
		out = append(out, byte(p>>24), byte(p>>16), byte(p>>8), byte(p))
	}
	return out
}

// ExtendedPubkey returns the serialized xpub (tpub on the test app) at path.
func (c *Client) ExtendedPubkey(path []uint32, display bool) (string, error) {
	d := byte(0)
	if display {
		d = 1
	}
	resp, err := c.run(insGetPubkey, append([]byte{d}, pathBytes(path)...), nil)
	if err != nil {
		return "", err
	}
	return string(resp), nil
}

// WalletAddress asks the device for the address of the policy at
// (change, index) and optionally shows it on the screen.
func (c *Client) WalletAddress(p Policy, change bool, index uint32, display bool) (string, error) {
	in := newInterpreter()
	in.addList(keyBytes(p.Keys))
	in.addPreimage(p.serialize())
	in.addPreimage([]byte(p.Template))

	id := p.id()
	data := []byte{0}
	if display {
		data[0] = 1
	}
	data = append(data, id[:]...)
	data = append(data, make([]byte, 32)...)
	if change {
		data = append(data, 1)
	} else {
		data = append(data, 0)
	}
	data = append(data, byte(index>>24), byte(index>>16), byte(index>>8), byte(index))
	resp, err := c.run(insGetWalletAddr, data, in)
	if err != nil {
		return "", err
	}
	return string(resp), nil
}

func keyBytes(keys []string) [][]byte {
	out := make([][]byte, len(keys))
	for i, k := range keys {
		out[i] = []byte(k)
	}
	return out
}

// PartialSig is one signature the device produced.
type PartialSig struct {
	Input  int
	PubKey []byte
	// Sig is DER followed by the sighash byte, as it goes in the witness.
	Sig []byte
}

// SignPSBT has the device sign every input of a version-2 PSBT that belongs
// to the policy.
func (c *Client) SignPSBT(global Map, inputs, outputs []Map, p Policy) ([]PartialSig, error) {
	in := newInterpreter()
	in.addList(keyBytes(p.Keys))
	in.addPreimage(p.serialize())
	in.addPreimage([]byte(p.Template))
	in.addMap(global)
	for _, m := range inputs {
		in.addMap(m)
	}
	for _, m := range outputs {
		in.addMap(m)
	}
	inCommit := make([][]byte, len(inputs))
	inLeaves := make([]hash32, len(inputs))
	for i, m := range inputs {
		inCommit[i] = m.commitment()
		inLeaves[i] = elementHash(inCommit[i])
	}
	outCommit := make([][]byte, len(outputs))
	outLeaves := make([]hash32, len(outputs))
	for i, m := range outputs {
		outCommit[i] = m.commitment()
		outLeaves[i] = elementHash(outCommit[i])
	}
	in.addList(inCommit)
	in.addList(outCommit)

	id := p.id()
	inRoot, outRoot := rootOf(inLeaves), rootOf(outLeaves)
	data := global.commitment()
	data = append(data, writeVarint(uint64(len(inputs)))...)
	data = append(data, inRoot[:]...)
	data = append(data, writeVarint(uint64(len(outputs)))...)
	data = append(data, outRoot[:]...)
	data = append(data, id[:]...)
	data = append(data, make([]byte, 32)...)

	if _, err := c.run(insSignPSBT, data, in); err != nil {
		return nil, err
	}
	var sigs []PartialSig
	for _, y := range in.yielded {
		s, err := parseYield(y)
		if err != nil {
			return nil, err
		}
		sigs = append(sigs, s)
	}
	return sigs, nil
}

func parseYield(y []byte) (PartialSig, error) {
	r := bytes.NewReader(y)
	idx, err := readVarint(r)
	if err != nil {
		return PartialSig{}, fmt.Errorf("ledger: bad yielded value: %w", err)
	}
	if idx >= 0x80000000 {
		return PartialSig{}, fmt.Errorf("ledger: unexpected yielded tag 0x%x (MuSig and taproot are not supported)", idx)
	}
	n, err := r.ReadByte()
	if err != nil || (n != 33) {
		return PartialSig{}, errors.New("ledger: yielded value without a compressed public key")
	}
	pub := make([]byte, n)
	if _, err := io.ReadFull(r, pub); err != nil {
		return PartialSig{}, err
	}
	sig, _ := io.ReadAll(r)
	return PartialSig{Input: int(idx), PubKey: pub, Sig: sig}, nil
}
