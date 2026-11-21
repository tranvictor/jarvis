package signer

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"github.com/tranvictor/jarvis/btc"
	"github.com/tranvictor/jarvis/btc/spend"
	"github.com/tranvictor/jarvis/btc/wallet"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
)

// TrezorCoinName is the coin definition the firmware knows the chain by.
func TrezorCoinName(c btc.Chain) string {
	switch c {
	case btc.Testnet, btc.Signet:
		return "Testnet"
	case btc.Regtest:
		return "Regtest"
	}
	return "Bitcoin"
}

const trezorAttempts = 3

type Trezor struct {
	Wallet *wallet.Wallet
	Src    TxSource
	// Notify receives progress lines (device prompts, retries).
	Notify func(format string, args ...any)
}

func (t *Trezor) notify(format string, args ...any) {
	if t.Notify != nil {
		t.Notify(format, args...)
	}
}

// Connect opens the device and proves it holds this wallet's account key.
// A passphrase that opens a different hidden wallet is retried, not trusted.
func (t *Trezor) Connect() (*trezoreum.BitcoinTrezor, error) {
	acctPath, err := t.Wallet.AccountPath()
	if err != nil {
		return nil, err
	}
	wantPub, wantChain, err := t.Wallet.AccountNode()
	if err != nil {
		return nil, err
	}
	coin := TrezorCoinName(t.Wallet.Chain)
	for attempt := 1; ; attempt++ {
		dev, err := trezoreum.NewBitcoinTrezor(t.Wallet.Rec.XPub)
		if err != nil {
			return nil, err
		}
		if err := dev.Connect(); err != nil {
			dev.Close()
			return nil, err
		}
		node, err := dev.PublicKey(acctPath, coin)
		if err != nil {
			dev.Close()
			return nil, wrapCancel(err)
		}
		if bytes.Equal(node.PublicKey, wantPub) && bytes.Equal(node.ChainCode, wantChain) {
			dev.BindPassphrase()
			return dev, nil
		}
		dev.ForgetPassphrase()
		dev.Close()
		if attempt >= trezorAttempts {
			return nil, fmt.Errorf("%w: this Trezor (with the passphrase given) does not hold wallet %q", ErrWrongWallet, t.Wallet.Rec.Name)
		}
		t.notify("This passphrase opens a different wallet than %q. Try again.", t.Wallet.Rec.Name)
	}
}

func wrapCancel(err error) error {
	if errors.Is(err, trezoreum.ErrCancelled) {
		return fmt.Errorf("%w: %v", ErrUserCancelled, err)
	}
	return err
}

func (t *Trezor) Sign(ctx context.Context, p *spend.Plan) (*wire.MsgTx, error) {
	acctPath, err := t.Wallet.AccountPath()
	if err != nil {
		return nil, err
	}
	req := trezoreum.BTCSignRequest{
		Coin:     TrezorCoinName(t.Wallet.Chain),
		Version:  uint32(p.Tx.Version),
		LockTime: p.Tx.LockTime,
		Prev:     map[string]trezoreum.BTCPrevTx{},
	}
	for i, c := range p.Inputs {
		req.Inputs = append(req.Inputs, trezoreum.BTCInput{
			Path:     append(append([]uint32{}, acctPath...), c.Branch, c.Index),
			TxID:     c.TxID,
			Index:    c.Vout,
			Sequence: p.Tx.TxIn[i].Sequence,
			Amount:   uint64(c.Value),
		})
		if _, ok := req.Prev[c.TxID]; ok {
			continue
		}
		prev, err := t.fetchPrev(ctx, c)
		if err != nil {
			return nil, err
		}
		req.Prev[c.TxID] = prev
	}
	for _, o := range p.Outputs {
		bo := trezoreum.BTCOutput{Address: o.Address, Amount: uint64(o.Value)}
		if o.Change {
			bo.ChangePath = append(append([]uint32{}, acctPath...), o.ChangeBranch, o.ChangeIndex)
		}
		req.Outputs = append(req.Outputs, bo)
	}

	dev, err := t.Connect()
	if err != nil {
		return nil, err
	}
	defer dev.Close()
	t.notify("Confirm the transaction on your Trezor.")
	res, err := dev.SignTx(req)
	if err != nil {
		return nil, wrapCancel(err)
	}

	tx := p.Tx.Copy()
	for i, c := range p.Inputs {
		der, err := toDER(res.Signatures[i])
		if err != nil {
			return nil, fmt.Errorf("input %d: %w", i, err)
		}
		tx.TxIn[i].Witness = wire.TxWitness{append(der, byte(txscript.SigHashAll)), c.PubKey}
	}
	return tx, nil
}

func (t *Trezor) fetchPrev(ctx context.Context, c wallet.Coin) (trezoreum.BTCPrevTx, error) {
	mt, err := fetchPrevTx(ctx, t.Src, c)
	if err != nil {
		return trezoreum.BTCPrevTx{}, err
	}
	out := trezoreum.BTCPrevTx{Version: uint32(mt.Version), LockTime: mt.LockTime}
	for _, in := range mt.TxIn {
		out.Inputs = append(out.Inputs, trezoreum.BTCPrevInput{
			TxID: in.PreviousOutPoint.Hash.String(), Index: in.PreviousOutPoint.Index,
			ScriptSig: in.SignatureScript, Sequence: in.Sequence,
		})
	}
	for _, o := range mt.TxOut {
		out.Outputs = append(out.Outputs, trezoreum.BTCPrevOutput{Amount: uint64(o.Value), ScriptPubKey: o.PkScript})
	}
	return out, nil
}

// fetchPrevTx downloads the transaction a coin lives in and checks it is the
// one the wallet scan described. Hardware devices trust it to learn what an
// input is worth, so a server must not be able to make one up.
func fetchPrevTx(ctx context.Context, src TxSource, c wallet.Coin) (*wire.MsgTx, error) {
	raw, err := src.TxHex(ctx, c.TxID)
	if err != nil {
		return nil, fmt.Errorf("fetching previous transaction %s: %w", c.TxID, err)
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("previous transaction %s: %w", c.TxID, err)
	}
	var mt wire.MsgTx
	if err := mt.Deserialize(bytes.NewReader(b)); err != nil {
		return nil, fmt.Errorf("previous transaction %s: %w", c.TxID, err)
	}
	if mt.TxHash().String() != c.TxID {
		return nil, fmt.Errorf("server returned transaction %s for %s", mt.TxHash(), c.TxID)
	}
	if int(c.Vout) >= len(mt.TxOut) || mt.TxOut[c.Vout].Value != c.Value || !bytes.Equal(mt.TxOut[c.Vout].PkScript, c.Script) {
		return nil, fmt.Errorf("previous transaction %s output %d does not match the coin the wallet scan reported", c.TxID, c.Vout)
	}
	return &mt, nil
}

// toDER accepts a DER signature or a raw 64-byte r||s one.
func toDER(sig []byte) ([]byte, error) {
	if len(sig) == 64 {
		var r, s btcec.ModNScalar
		if r.SetByteSlice(sig[:32]) || s.SetByteSlice(sig[32:]) {
			return nil, errors.New("device returned an out-of-range signature")
		}
		return ecdsa.NewSignature(&r, &s).Serialize(), nil
	}
	if _, err := ecdsa.ParseDERSignature(sig); err != nil {
		return nil, fmt.Errorf("device returned an unusable signature: %w", err)
	}
	return sig, nil
}

// Discovery is what `wallet add` learns from a device about one account.
type Discovery struct {
	XPub         string
	MasterFP     string
	FirstAddress string
}

// DiscoverTrezor reads the account key of a Trezor and has the operator
// compare the first receive address on the device screen with the one
// Jarvis derives from the key. A mismatch aborts: it would mean Jarvis and
// the device disagree on what the wallet is.
func DiscoverTrezor(chain btc.Chain, acctPath []uint32, notify func(string, ...any)) (*Discovery, error) {
	dev, err := trezoreum.NewBitcoinTrezor("add:" + btc.FormatPath(acctPath))
	if err != nil {
		return nil, err
	}
	defer dev.Close()
	if err := dev.Connect(); err != nil {
		return nil, err
	}
	coin := TrezorCoinName(chain)
	node, err := dev.PublicKey(acctPath, coin)
	if err != nil {
		return nil, wrapCancel(err)
	}
	key, err := wallet.ParseXPub(node.XPub, chain.Params())
	if err != nil {
		return nil, err
	}
	probe, err := wallet.Open(wallet.Record{Name: "probe", Kind: wallet.KindTrezor, Chain: string(chain), AccountPath: btc.FormatPath(acctPath), XPub: key.String()})
	if err != nil {
		return nil, err
	}
	first, err := probe.Derive(0, 0)
	if err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Compare this address with the one on your Trezor screen:\n  %s", first.Address)
	}
	shown, err := dev.Address(append(append([]uint32{}, acctPath...), 0, 0), coin, true)
	if err != nil {
		return nil, wrapCancel(err)
	}
	if shown != first.Address {
		return nil, fmt.Errorf("the Trezor derived %s but Jarvis derived %s from its key; refusing to add the wallet", shown, first.Address)
	}
	d := &Discovery{XPub: key.String(), FirstAddress: first.Address}
	if node.RootFingerprint != 0 {
		d.MasterFP = fmt.Sprintf("%08x", node.RootFingerprint)
	}
	return d, nil
}
