package signer

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"github.com/tranvictor/jarvis/btc"
	"github.com/tranvictor/jarvis/btc/ledgerbtc"
	"github.com/tranvictor/jarvis/btc/spend"
	"github.com/tranvictor/jarvis/btc/wallet"
)

// LedgerAppName is the application the device must be running: the main
// Bitcoin app for mainnet, "Bitcoin Test" for every other chain.
func LedgerAppName(c btc.Chain) string {
	if c == btc.Mainnet {
		return "Bitcoin"
	}
	return "Bitcoin Test"
}

const ledgerWaitDefault = 90 * time.Second

type Ledger struct {
	Wallet *wallet.Wallet
	Src    TxSource
	Notify func(format string, args ...any)
}

func (l *Ledger) notify(format string, args ...any) {
	if l.Notify != nil {
		l.Notify(format, args...)
	}
}

func deviceWait() time.Duration {
	if v := os.Getenv("JARVIS_BTC_DEVICE_WAIT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return ledgerWaitDefault
}

// openLedger waits for a device with the right Bitcoin app open.
func openLedger(chain btc.Chain, notify func(string, ...any)) (*ledgerbtc.Client, ledgerbtc.Transport, error) {
	deadline := time.Now().Add(deviceWait())
	announced := false
	for {
		tr, err := ledgerbtc.Open()
		if err == nil {
			cl := &ledgerbtc.Client{T: tr}
			name, version, verr := cl.Version()
			if verr == nil && name == LedgerAppName(chain) {
				return cl, tr, nil
			}
			tr.Close()
			err = verr
			if verr == nil {
				err = fmt.Errorf("the Ledger is running %q %s; open the %q app", name, version, LedgerAppName(chain))
			}
			if !time.Now().Before(deadline) {
				return nil, nil, err
			}
		} else if !errors.Is(err, ledgerbtc.ErrNoDevice) || !time.Now().Before(deadline) {
			return nil, nil, err
		}
		if !announced && notify != nil {
			notify("Connect your Ledger, unlock it and open the %q app…", LedgerAppName(chain))
			announced = true
		}
		time.Sleep(2 * time.Second)
	}
}

func wrapLedger(err error) error {
	if errors.Is(err, ledgerbtc.ErrDenied) {
		return fmt.Errorf("%w: %v", ErrUserCancelled, err)
	}
	return err
}

func keyInfo(fp []byte, acct []uint32, xpub string) string {
	return fmt.Sprintf("[%x/%s]%s", fp, btc.FormatPath(acct)[2:], xpub)
}

func (l *Ledger) Sign(ctx context.Context, p *spend.Plan) (*wire.MsgTx, error) {
	acct, err := l.Wallet.AccountPath()
	if err != nil {
		return nil, err
	}
	fp, err := hex.DecodeString(l.Wallet.Rec.MasterFP)
	if err != nil || len(fp) != 4 {
		return nil, errors.New("this wallet record has no master fingerprint; remove it and add the Ledger again")
	}
	wantPub, wantChain, err := l.Wallet.AccountNode()
	if err != nil {
		return nil, err
	}

	prev := make([]*wire.MsgTx, len(p.Inputs))
	for i, c := range p.Inputs {
		if prev[i], err = fetchPrevTx(ctx, l.Src, c); err != nil {
			return nil, err
		}
	}

	cl, tr, err := openLedger(l.Wallet.Chain, l.notify)
	if err != nil {
		return nil, err
	}
	defer tr.Close()

	devFP, err := cl.MasterFingerprint()
	if err != nil {
		return nil, wrapLedger(err)
	}
	if !bytes.Equal(devFP[:], fp) {
		return nil, fmt.Errorf("%w: this Ledger has master fingerprint %x, wallet %q was created from %x",
			ErrWrongWallet, devFP[:], l.Wallet.Rec.Name, fp)
	}
	devXPub, err := cl.ExtendedPubkey(acct, false)
	if err != nil {
		return nil, wrapLedger(err)
	}
	devKey, err := wallet.ParseXPub(devXPub, l.Wallet.Chain.Params())
	if err != nil {
		return nil, err
	}
	devPub, err := devKey.ECPubKey()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(devPub.SerializeCompressed(), wantPub) || !bytes.Equal(devKey.ChainCode(), wantChain) {
		return nil, fmt.Errorf("%w: this Ledger's account %s differs from wallet %q", ErrWrongWallet, btc.FormatPath(acct), l.Wallet.Rec.Name)
	}

	global := ledgerbtc.Map{}
	global.Add([]byte{0x02}, le32(uint32(p.Tx.Version)))
	global.Add([]byte{0x03}, le32(p.Tx.LockTime))
	global.Add([]byte{0x04}, varint(len(p.Inputs)))
	global.Add([]byte{0x05}, varint(len(p.Outputs)))
	global.Add([]byte{0xfb}, le32(2))

	inputs := make([]ledgerbtc.Map, len(p.Inputs))
	for i, c := range p.Inputs {
		hash, err := chainhash.NewHashFromStr(c.TxID)
		if err != nil {
			return nil, err
		}
		var raw bytes.Buffer
		if err := prev[i].SerializeNoWitness(&raw); err != nil {
			return nil, err
		}
		var wit bytes.Buffer
		if err := wire.WriteTxOut(&wit, 0, 0, &wire.TxOut{Value: c.Value, PkScript: c.Script}); err != nil {
			return nil, err
		}
		m := ledgerbtc.Map{}
		m.Add([]byte{0x00}, raw.Bytes())
		m.Add([]byte{0x01}, wit.Bytes())
		m.Add([]byte{0x03}, le32(uint32(txscript.SigHashAll)))
		m.Add(append([]byte{0x06}, c.PubKey...), origin(fp, acct, c.Branch, c.Index))
		m.Add([]byte{0x0e}, hash[:])
		m.Add([]byte{0x0f}, le32(c.Vout))
		m.Add([]byte{0x10}, le32(p.Tx.TxIn[i].Sequence))
		inputs[i] = m
	}
	outputs := make([]ledgerbtc.Map, len(p.Outputs))
	for i, o := range p.Outputs {
		m := ledgerbtc.Map{}
		amount := make([]byte, 8)
		binary.LittleEndian.PutUint64(amount, uint64(o.Value))
		m.Add([]byte{0x03}, amount)
		m.Add([]byte{0x04}, o.Script)
		if o.Change {
			d, err := l.Wallet.Derive(o.ChangeBranch, o.ChangeIndex)
			if err != nil {
				return nil, err
			}
			m.Add(append([]byte{0x02}, d.PubKey...), origin(fp, acct, o.ChangeBranch, o.ChangeIndex))
		}
		outputs[i] = m
	}

	policy := ledgerbtc.DefaultWPKH(keyInfo(fp, acct, l.Wallet.Rec.XPub))
	l.notify("Review and approve the transaction on your Ledger.")
	sigs, err := cl.SignPSBT(global, inputs, outputs, policy)
	if err != nil {
		return nil, wrapLedger(err)
	}

	tx := p.Tx.Copy()
	seen := map[int]bool{}
	for _, s := range sigs {
		if s.Input < 0 || s.Input >= len(p.Inputs) {
			return nil, fmt.Errorf("ledger signed input %d of a %d-input transaction", s.Input, len(p.Inputs))
		}
		if seen[s.Input] {
			return nil, fmt.Errorf("ledger signed input %d twice", s.Input)
		}
		seen[s.Input] = true
		c := p.Inputs[s.Input]
		if !bytes.Equal(s.PubKey, c.PubKey) {
			return nil, fmt.Errorf("ledger signed input %d with an unexpected key", s.Input)
		}
		if len(s.Sig) < 2 || s.Sig[len(s.Sig)-1] != byte(txscript.SigHashAll) {
			return nil, fmt.Errorf("ledger returned a signature without SIGHASH_ALL for input %d", s.Input)
		}
		if _, err := ecdsa.ParseDERSignature(s.Sig[:len(s.Sig)-1]); err != nil {
			return nil, fmt.Errorf("ledger returned an unusable signature for input %d: %w", s.Input, err)
		}
		tx.TxIn[s.Input].Witness = wire.TxWitness{s.Sig, c.PubKey}
	}
	for i := range p.Inputs {
		if !seen[i] {
			return nil, fmt.Errorf("ledger did not sign input %d", i)
		}
	}
	return tx, nil
}

func le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func varint(n int) []byte {
	var b bytes.Buffer
	_ = wire.WriteVarInt(&b, 0, uint64(n))
	return b.Bytes()
}

// origin is a PSBT key origin: fingerprint followed by the path, each
// element a little-endian uint32.
func origin(fp []byte, acct []uint32, branch, index uint32) []byte {
	out := append([]byte(nil), fp...)
	for _, e := range append(append([]uint32{}, acct...), branch, index) {
		out = append(out, le32(e)...)
	}
	return out
}

// DiscoverLedger reads the account key of a Ledger and has the operator
// compare the first receive address on the device with the one Jarvis
// derives from the key.
func DiscoverLedger(chain btc.Chain, acctPath []uint32, notify func(string, ...any)) (*Discovery, error) {
	cl, tr, err := openLedger(chain, notify)
	if err != nil {
		return nil, err
	}
	defer tr.Close()

	fp, err := cl.MasterFingerprint()
	if err != nil {
		return nil, wrapLedger(err)
	}
	xpub, err := cl.ExtendedPubkey(acctPath, false)
	if err != nil {
		return nil, wrapLedger(err)
	}
	key, err := wallet.ParseXPub(xpub, chain.Params())
	if err != nil {
		return nil, err
	}
	probe, err := wallet.Open(wallet.Record{Name: "probe", Kind: wallet.KindLedger, Chain: string(chain), AccountPath: btc.FormatPath(acctPath), XPub: key.String()})
	if err != nil {
		return nil, err
	}
	first, err := probe.Derive(0, 0)
	if err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Compare this address with the one on your Ledger screen:\n  %s", first.Address)
	}
	shown, err := cl.WalletAddress(ledgerbtc.DefaultWPKH(keyInfo(fp[:], acctPath, key.String())), false, 0, true)
	if err != nil {
		return nil, wrapLedger(err)
	}
	if err := sameAddress(chain, shown, first); err != nil {
		return nil, err
	}
	return &Discovery{XPub: key.String(), MasterFP: hex.EncodeToString(fp[:]), FirstAddress: first.Address}, nil
}

// sameAddress compares the address a Ledger reports with the one derived on
// the host. The Bitcoin Test app only knows the testnet prefix, so for
// signet and regtest the two strings differ even when they name the same
// output; the scripts are compared instead.
func sameAddress(chain btc.Chain, shown string, want wallet.Derived) error {
	params := chain.Params()
	if chain != btc.Mainnet {
		params = btc.Testnet.Params()
	}
	addr, err := btc.DecodeAddress(shown, params)
	if err != nil {
		return fmt.Errorf("the Ledger returned an address Jarvis cannot read (%s): %w", shown, err)
	}
	script, err := btc.PayTo(addr)
	if err != nil {
		return err
	}
	if !bytes.Equal(script, want.Script) {
		return fmt.Errorf("the Ledger derived %s but Jarvis derived %s from its key; refusing to add the wallet", shown, want.Address)
	}
	return nil
}
