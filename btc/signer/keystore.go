package signer

import (
	"context"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"

	"github.com/tranvictor/jarvis/btc/spend"
	"github.com/tranvictor/jarvis/btc/wallet"
	"github.com/tranvictor/jarvis/util/account"
)

type Keystore struct {
	Wallet   *wallet.Wallet
	Path     string
	Password func() (string, error)
}

func (k *Keystore) Sign(ctx context.Context, p *spend.Plan) (*wire.MsgTx, error) {
	pw, err := k.Password()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserCancelled, err)
	}
	_, ecKey, err := account.PrivateKeyFromKeystore(k.Path, pw)
	if err != nil {
		return nil, fmt.Errorf("unlocking keystore %s: %w", k.Path, err)
	}
	d := make([]byte, 32)
	ecKey.D.FillBytes(d)
	priv, _ := btcec.PrivKeyFromBytes(d)
	for i := range d {
		d[i] = 0
	}

	own, err := k.Wallet.Derive(0, 0)
	if err != nil {
		return nil, err
	}
	if string(priv.PubKey().SerializeCompressed()) != string(own.PubKey) {
		return nil, fmt.Errorf("%w: keystore %s unlocks a different key than wallet %q", ErrWrongWallet, k.Path, k.Wallet.Rec.Name)
	}

	// BIP-143 commits to the P2PKH script of the key hash as scriptCode.
	pkh, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(own.PubKey), k.Wallet.Params)
	if err != nil {
		return nil, err
	}
	scriptCode, err := txscript.PayToAddrScript(pkh)
	if err != nil {
		return nil, err
	}

	tx := p.Tx.Copy()
	prev := txscript.NewMultiPrevOutFetcher(nil)
	for i, c := range p.Inputs {
		prev.AddPrevOut(tx.TxIn[i].PreviousOutPoint, wire.NewTxOut(c.Value, c.Script))
	}
	sigHashes := txscript.NewTxSigHashes(tx, prev)
	for i, c := range p.Inputs {
		witness, err := txscript.WitnessSignature(tx, sigHashes, i, c.Value, scriptCode, txscript.SigHashAll, priv, true)
		if err != nil {
			return nil, fmt.Errorf("signing input %d: %w", i, err)
		}
		tx.TxIn[i].Witness = witness
	}
	return tx, nil
}
