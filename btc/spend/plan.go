// Package spend turns a wallet scan plus a payment request into an unsigned
// transaction, and checks a signed one before it is broadcast.
package spend

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"

	"github.com/tranvictor/jarvis/btc"
	"github.com/tranvictor/jarvis/btc/wallet"
)

var (
	ErrInsufficient = errors.New("insufficient funds")
	ErrDust         = errors.New("amount is below the dust limit")
)

// SequenceRBF opts every input into BIP-125 replacement so a stuck payment
// can be fee-bumped later.
const SequenceRBF = 0xfffffffd

type Output struct {
	Address string
	Script  []byte
	Value   int64
	Change  bool
	// ChangeBranch and ChangeIndex locate a change output under the wallet's
	// account so a hardware device can recognise it as its own.
	ChangeBranch uint32
	ChangeIndex  uint32
}

type Plan struct {
	Wallet  *wallet.Wallet
	Inputs  []wallet.Coin
	Outputs []Output
	Fee     int64
	FeeRate float64
	// VSize is the size the fee was computed from. It is an upper bound: it
	// counts every signature at 72 bytes.
	VSize int
	Tx    *wire.MsgTx
}

type Request struct {
	Wallet           *wallet.Wallet
	Scan             *wallet.Scan
	To               btcutil.Address
	Amount           int64
	Sweep            bool
	FeeRate          float64
	SpendUnconfirmed bool
}

func feeFor(rate float64, vsize int) int64 {
	return int64(math.Ceil(rate * float64(vsize)))
}

// unconfirmedHint tells the user when coins they cannot spend yet exist.
func unconfirmedHint(req Request) string {
	if req.SpendUnconfirmed {
		return ""
	}
	var sum int64
	for _, c := range req.Scan.Coins {
		if !c.Confirmed {
			sum += c.Value
		}
	}
	if sum == 0 {
		return ""
	}
	return fmt.Sprintf("%s BTC more is unconfirmed (wait for a confirmation or pass --spend-unconfirmed)", btc.FormatBTC(sum))
}

func eligible(req Request) []wallet.Coin {
	var out []wallet.Coin
	for _, c := range req.Scan.Coins {
		if c.Confirmed || req.SpendUnconfirmed {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		if out[i].TxID != out[j].TxID {
			return out[i].TxID < out[j].TxID
		}
		return out[i].Vout < out[j].Vout
	})
	return out
}

func Build(req Request) (*Plan, error) {
	if req.FeeRate < 1 {
		return nil, fmt.Errorf("fee rate %.2f sat/vB is below the 1 sat/vB relay minimum", req.FeeRate)
	}
	if !req.Sweep && req.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", btc.ErrAmount)
	}
	destScript, err := btc.PayTo(req.To)
	if err != nil {
		return nil, err
	}
	coins := eligible(req)
	if len(coins) == 0 {
		msg := "the wallet has no confirmed coins to spend"
		if req.Scan.Pending != 0 {
			msg += " (a payment is still unconfirmed)"
		}
		return nil, fmt.Errorf("%w: %s", ErrInsufficient, msg)
	}

	var (
		inputs  []wallet.Coin
		outputs []Output
		fee     int64
		vsize   int
	)
	dest := Output{Address: req.To.EncodeAddress(), Script: destScript}

	if req.Sweep {
		var sum int64
		for _, c := range coins {
			sum += c.Value
		}
		vsize = btc.EstimateVSize(len(coins), [][]byte{destScript})
		fee = feeFor(req.FeeRate, vsize)
		dest.Value = sum - fee
		if dest.Value <= 0 {
			return nil, fmt.Errorf("%w: all coins (%s BTC) cannot pay the %s BTC fee", ErrInsufficient, btc.FormatBTC(sum), btc.FormatBTC(fee))
		}
		if dest.Value < btc.DustThreshold(destScript) {
			return nil, fmt.Errorf("%w: sweeping leaves %d sat, below %d", ErrDust, dest.Value, btc.DustThreshold(destScript))
		}
		inputs, outputs = coins, []Output{dest}
	} else {
		if req.Amount < btc.DustThreshold(destScript) {
			return nil, fmt.Errorf("%w: %d sat is below %d sat for this address type", ErrDust, req.Amount, btc.DustThreshold(destScript))
		}
		if req.Amount > btc.MaxMoney {
			return nil, fmt.Errorf("%w: above 21,000,000 BTC", btc.ErrAmount)
		}
		dest.Value = req.Amount
		chg, err := changeOutput(req)
		if err != nil {
			return nil, err
		}
		var sum int64
		for i, c := range coins {
			sum += c.Value
			inputs = coins[:i+1]
			withChange := feeFor(req.FeeRate, btc.EstimateVSize(len(inputs), [][]byte{destScript, chg.Script}))
			if left := sum - req.Amount - withChange; left >= btc.DustThreshold(chg.Script) {
				chg.Value = left
				outputs = []Output{dest, chg}
				fee, vsize = withChange, btc.EstimateVSize(len(inputs), [][]byte{destScript, chg.Script})
				break
			}
			noChangeVSize := btc.EstimateVSize(len(inputs), [][]byte{destScript})
			if sum-req.Amount >= feeFor(req.FeeRate, noChangeVSize) {
				// Whatever is left after the payment is too small to be worth
				// a change output, so it goes to the miner.
				outputs = []Output{dest}
				fee, vsize = sum-req.Amount, noChangeVSize
				break
			}
		}
		if outputs == nil {
			msg := fmt.Sprintf("need %s BTC plus fee, have %s BTC spendable", btc.FormatBTC(req.Amount), btc.FormatBTC(sum))
			if hint := unconfirmedHint(req); hint != "" {
				msg += "; " + hint
			}
			return nil, fmt.Errorf("%w: %s", ErrInsufficient, msg)
		}
	}

	tx := wire.NewMsgTx(2)
	for _, c := range inputs {
		h, err := chainhash.NewHashFromStr(c.TxID)
		if err != nil {
			return nil, fmt.Errorf("coin %s:%d: %w", c.TxID, c.Vout, err)
		}
		in := wire.NewTxIn(wire.NewOutPoint(h, c.Vout), nil, nil)
		in.Sequence = SequenceRBF
		tx.AddTxIn(in)
	}
	for _, o := range outputs {
		tx.AddTxOut(wire.NewTxOut(o.Value, o.Script))
	}
	return &Plan{
		Wallet: req.Wallet, Inputs: inputs, Outputs: outputs,
		Fee: fee, FeeRate: req.FeeRate, VSize: vsize, Tx: tx,
	}, nil
}

func changeOutput(req Request) (Output, error) {
	idx := req.Scan.NextChange
	d, err := req.Wallet.Derive(1, idx)
	if err != nil {
		return Output{}, err
	}
	return Output{Address: d.Address, Script: d.Script, Change: true, ChangeBranch: d.Branch, ChangeIndex: d.Index}, nil
}

func (p *Plan) Total() (in int64) {
	for _, c := range p.Inputs {
		in += c.Value
	}
	return in
}
