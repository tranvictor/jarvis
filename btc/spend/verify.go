package spend

import (
	"bytes"
	"fmt"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// VSize returns the virtual size of a fully signed transaction.
func VSize(tx *wire.MsgTx) int {
	stripped := tx.SerializeSizeStripped()
	weight := stripped*3 + tx.SerializeSize()
	return (weight + 3) / 4
}

// Verify checks a signed transaction against the plan it was built from
// and runs every input through the consensus script engine. It exists so a
// device bug, a wrong passphrase or a wrong key is caught here and not by a
// rejected broadcast, or worse, by an accepted one.
func (p *Plan) Verify(signed *wire.MsgTx) error {
	if len(signed.TxIn) != len(p.Tx.TxIn) || len(signed.TxOut) != len(p.Tx.TxOut) {
		return fmt.Errorf("signed transaction has %d inputs and %d outputs, expected %d and %d",
			len(signed.TxIn), len(signed.TxOut), len(p.Tx.TxIn), len(p.Tx.TxOut))
	}
	if signed.Version != p.Tx.Version || signed.LockTime != p.Tx.LockTime {
		return fmt.Errorf("signed transaction changed version or locktime")
	}
	for i, in := range signed.TxIn {
		if in.PreviousOutPoint != p.Tx.TxIn[i].PreviousOutPoint {
			return fmt.Errorf("input %d spends %s, expected %s", i, in.PreviousOutPoint, p.Tx.TxIn[i].PreviousOutPoint)
		}
		if in.Sequence != p.Tx.TxIn[i].Sequence {
			return fmt.Errorf("input %d changed its sequence", i)
		}
		if len(in.SignatureScript) != 0 {
			return fmt.Errorf("input %d has a scriptSig; native segwit inputs must not", i)
		}
	}
	for i, out := range signed.TxOut {
		want := p.Tx.TxOut[i]
		if out.Value != want.Value || !bytes.Equal(out.PkScript, want.PkScript) {
			return fmt.Errorf("output %d was altered between the plan and the signature", i)
		}
	}

	prev := txscript.NewMultiPrevOutFetcher(nil)
	for i, c := range p.Inputs {
		prev.AddPrevOut(signed.TxIn[i].PreviousOutPoint, wire.NewTxOut(c.Value, c.Script))
	}
	sigHashes := txscript.NewTxSigHashes(signed, prev)
	for i, c := range p.Inputs {
		vm, err := txscript.NewEngine(c.Script, signed, i, txscript.StandardVerifyFlags, nil, sigHashes, c.Value, prev)
		if err != nil {
			return fmt.Errorf("input %d: %w", i, err)
		}
		if err := vm.Execute(); err != nil {
			return fmt.Errorf("input %d does not carry a valid signature for %s: %w", i, c.Address, err)
		}
	}

	var out int64
	for _, o := range signed.TxOut {
		out += o.Value
	}
	if fee := p.Total() - out; fee != p.Fee {
		return fmt.Errorf("signed transaction pays %d sat in fees, the plan said %d", fee, p.Fee)
	}
	if rate := float64(p.Fee) / float64(VSize(signed)); rate < 1 {
		return fmt.Errorf("effective fee rate %.2f sat/vB is below the relay minimum", rate)
	}
	return nil
}
