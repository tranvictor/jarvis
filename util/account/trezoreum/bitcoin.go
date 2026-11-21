package trezoreum

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/golang/protobuf/proto"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
)

// BitcoinTrezor drives a Trezor through the Bitcoin application messages.
// It shares the USB transport, PIN and passphrase handling with the Ethereum
// signer but nothing else: it never issues an Ethereum message.
type BitcoinTrezor struct {
	dev *Trezoreum
	// guard reuses TrezorSigner's Ctrl-C handling and failure reset.
	guard *TrezorSigner
	// key identifies this wallet in the process-wide passphrase cache. The
	// cache is keyed by an Ethereum address for the Ethereum signer; a hash of
	// the account xpub plays that role here so the two never collide.
	key common.Address
}

func NewBitcoinTrezor(walletID string) (*BitcoinTrezor, error) {
	dev, err := NewTrezoreum()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte("jarvis-btc:" + walletID))
	key := common.BytesToAddress(sum[:20])
	dev.SetExpectedAddress(key)
	return &BitcoinTrezor{
		dev:   dev,
		guard: &TrezorSigner{trezor: dev, address: key},
		key:   key,
	}, nil
}

// Connect waits for the device, then walks the PIN and passphrase prompts.
func (b *BitcoinTrezor) Connect() error {
	wait := deviceWaitTimeout
	if v := os.Getenv("JARVIS_BTC_DEVICE_WAIT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			wait = d
		}
	}
	return unlockWithWait(b.dev.Unlock, wait)
}

func (b *BitcoinTrezor) Close() { _ = b.dev.Close() }

// ForgetPassphrase drops the cached passphrase of this wallet so the next
// Connect asks again. Used when a passphrase opened the wrong hidden wallet.
func (b *BitcoinTrezor) ForgetPassphrase() { ResetPassphraseSession(b.key) }

// BindPassphrase remembers the passphrase that just produced the right
// account so a second command in the same process does not ask again.
func (b *BitcoinTrezor) BindPassphrase() { bindPassphrase(b.key) }

func (b *BitcoinTrezor) run(fn func() error) error {
	err := b.guard.runWithAbort(fn)
	if err != nil {
		b.dev.ResetAfterFailure()
	}
	return err
}

type BitcoinNode struct {
	PublicKey []byte
	ChainCode []byte
	XPub      string
	// RootFingerprint is the BIP-32 master fingerprint; 0 when the firmware
	// does not report it.
	RootFingerprint uint32
}

func (b *BitcoinTrezor) PublicKey(path []uint32, coin string) (*BitcoinNode, error) {
	var res *trezor.PublicKey
	err := b.run(func() error {
		res = new(trezor.PublicKey)
		_, err := b.dev.trezorExchange(&trezor.GetPublicKey{
			AddressN:        path,
			CoinName:        proto.String(coin),
			ScriptType:      trezor.InputScriptType_SPENDWITNESS.Enum(),
			IgnoreXpubMagic: proto.Bool(true),
		}, res)
		return err
	})
	if err != nil {
		return nil, err
	}
	node := res.GetNode()
	if node == nil {
		return nil, errors.New("trezor returned no public key node")
	}
	return &BitcoinNode{
		PublicKey:       node.GetPublicKey(),
		ChainCode:       node.GetChainCode(),
		XPub:            res.GetXpub(),
		RootFingerprint: res.GetRootFingerprint(),
	}, nil
}

// Address asks the device for the P2WPKH address at path, optionally
// showing it on the screen for the operator to compare.
func (b *BitcoinTrezor) Address(path []uint32, coin string, show bool) (string, error) {
	var res *trezor.Address
	err := b.run(func() error {
		res = new(trezor.Address)
		_, err := b.dev.trezorExchange(&trezor.GetAddress{
			AddressN:    path,
			CoinName:    proto.String(coin),
			ScriptType:  trezor.InputScriptType_SPENDWITNESS.Enum(),
			ShowDisplay: proto.Bool(show),
		}, res)
		return err
	})
	if err != nil {
		return "", err
	}
	return res.GetAddress(), nil
}

type BTCInput struct {
	Path []uint32
	// TxID is the previous transaction id in display (big-endian) hex order,
	// which is what the Trezor wire protocol uses.
	TxID     string
	Index    uint32
	Sequence uint32
	Amount   uint64
}

type BTCOutput struct {
	Address string
	// ChangePath, when set, makes the device treat the output as its own
	// change instead of showing it as a payment.
	ChangePath []uint32
	Amount     uint64
}

type BTCPrevInput struct {
	TxID      string
	Index     uint32
	ScriptSig []byte
	Sequence  uint32
}

type BTCPrevOutput struct {
	Amount       uint64
	ScriptPubKey []byte
}

type BTCPrevTx struct {
	Version  uint32
	LockTime uint32
	Inputs   []BTCPrevInput
	Outputs  []BTCPrevOutput
}

type BTCSignRequest struct {
	Coin     string
	Version  uint32
	LockTime uint32
	Inputs   []BTCInput
	Outputs  []BTCOutput
	// Prev maps a previous transaction id (display-order hex) to its content.
	Prev map[string]BTCPrevTx
}

type BTCSignResult struct {
	// Signatures holds one signature per input, as the device sent it
	// (DER without a sighash byte).
	Signatures [][]byte
	// Serialized is the full signed transaction the device assembled.
	Serialized []byte
}

func (b *BitcoinTrezor) SignTx(req BTCSignRequest) (*BTCSignResult, error) {
	var out *BTCSignResult
	err := b.run(func() error {
		var err error
		out, err = b.signTx(req)
		return err
	})
	return out, err
}

func txidBytes(txid string) ([]byte, error) {
	raw, err := hex.DecodeString(txid)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("bad txid %q", txid)
	}
	return raw, nil
}

func (b *BitcoinTrezor) signTx(req BTCSignRequest) (*BTCSignResult, error) {
	inputs := make([]*trezor.TxAck_TransactionType_TxInputType, len(req.Inputs))
	for i, in := range req.Inputs {
		h, err := txidBytes(in.TxID)
		if err != nil {
			return nil, err
		}
		inputs[i] = &trezor.TxAck_TransactionType_TxInputType{
			AddressN:   in.Path,
			PrevHash:   h,
			PrevIndex:  proto.Uint32(in.Index),
			Sequence:   proto.Uint32(in.Sequence),
			ScriptType: trezor.InputScriptType_SPENDWITNESS.Enum(),
			Amount:     proto.Uint64(in.Amount),
		}
	}
	outputs := make([]*trezor.TxAck_TransactionType_TxOutputType, len(req.Outputs))
	for i, o := range req.Outputs {
		to := &trezor.TxAck_TransactionType_TxOutputType{Amount: proto.Uint64(o.Amount)}
		if len(o.ChangePath) > 0 {
			to.AddressN = o.ChangePath
			to.ScriptType = trezor.OutputScriptType_PAYTOWITNESS.Enum()
		} else {
			to.Address = proto.String(o.Address)
			to.ScriptType = trezor.OutputScriptType_PAYTOADDRESS.Enum()
		}
		outputs[i] = to
	}

	start := &trezor.SignTx{
		OutputsCount: proto.Uint32(uint32(len(outputs))),
		InputsCount:  proto.Uint32(uint32(len(inputs))),
		CoinName:     proto.String(req.Coin),
		Version:      proto.Uint32(req.Version),
		LockTime:     proto.Uint32(req.LockTime),
	}
	res := new(trezor.TxRequest)
	if _, err := b.dev.trezorExchange(start, res); err != nil {
		return nil, err
	}

	result := &BTCSignResult{Signatures: make([][]byte, len(inputs))}
	for {
		if s := res.GetSerialized(); s != nil {
			result.Serialized = append(result.Serialized, s.GetSerializedTx()...)
			if s.SignatureIndex != nil {
				idx := int(s.GetSignatureIndex())
				if idx < 0 || idx >= len(inputs) {
					return nil, fmt.Errorf("trezor signed input %d of %d", idx, len(inputs))
				}
				if result.Signatures[idx] != nil {
					return nil, fmt.Errorf("trezor signed input %d twice", idx)
				}
				result.Signatures[idx] = append([]byte(nil), s.GetSignature()...)
			}
		}
		if res.GetRequestType() == trezor.TxRequest_TXFINISHED {
			break
		}
		details := res.GetDetails()
		if details == nil {
			return nil, errors.New("trezor asked for transaction data without details")
		}

		ack := &trezor.TxAck_TransactionType{}
		idx := int(details.GetRequestIndex())
		var prev *BTCPrevTx
		if len(details.GetTxHash()) > 0 {
			p, ok := req.Prev[hex.EncodeToString(details.GetTxHash())]
			if !ok {
				return nil, fmt.Errorf("trezor asked for unknown previous transaction %x", details.GetTxHash())
			}
			prev = &p
		}
		switch res.GetRequestType() {
		case trezor.TxRequest_TXMETA:
			if prev == nil {
				ack.Version = proto.Uint32(req.Version)
				ack.LockTime = proto.Uint32(req.LockTime)
				ack.InputsCnt = proto.Uint32(uint32(len(inputs)))
				ack.OutputsCnt = proto.Uint32(uint32(len(outputs)))
			} else {
				ack.Version = proto.Uint32(prev.Version)
				ack.LockTime = proto.Uint32(prev.LockTime)
				ack.InputsCnt = proto.Uint32(uint32(len(prev.Inputs)))
				ack.OutputsCnt = proto.Uint32(uint32(len(prev.Outputs)))
			}
		case trezor.TxRequest_TXINPUT:
			if prev == nil {
				if idx >= len(inputs) {
					return nil, fmt.Errorf("trezor asked for input %d of %d", idx, len(inputs))
				}
				ack.Inputs = []*trezor.TxAck_TransactionType_TxInputType{inputs[idx]}
			} else {
				if idx >= len(prev.Inputs) {
					return nil, fmt.Errorf("trezor asked for previous input %d of %d", idx, len(prev.Inputs))
				}
				pi := prev.Inputs[idx]
				h, err := txidBytes(pi.TxID)
				if err != nil {
					return nil, err
				}
				ack.Inputs = []*trezor.TxAck_TransactionType_TxInputType{{
					PrevHash:  h,
					PrevIndex: proto.Uint32(pi.Index),
					ScriptSig: pi.ScriptSig,
					Sequence:  proto.Uint32(pi.Sequence),
				}}
			}
		case trezor.TxRequest_TXOUTPUT:
			if prev == nil {
				if idx >= len(outputs) {
					return nil, fmt.Errorf("trezor asked for output %d of %d", idx, len(outputs))
				}
				ack.Outputs = []*trezor.TxAck_TransactionType_TxOutputType{outputs[idx]}
			} else {
				if idx >= len(prev.Outputs) {
					return nil, fmt.Errorf("trezor asked for previous output %d of %d", idx, len(prev.Outputs))
				}
				po := prev.Outputs[idx]
				ack.BinOutputs = []*trezor.TxAck_TransactionType_TxOutputBinType{{
					Amount:       proto.Uint64(po.Amount),
					ScriptPubkey: po.ScriptPubKey,
				}}
			}
		default:
			return nil, fmt.Errorf("trezor sent an unsupported signing request: %s", res.GetRequestType())
		}
		res = new(trezor.TxRequest)
		if _, err := b.dev.trezorExchange(&trezor.TxAck{Tx: ack}, res); err != nil {
			return nil, err
		}
	}
	for i, s := range result.Signatures {
		if s == nil {
			return nil, fmt.Errorf("trezor finished without signing input %d", i)
		}
	}
	return result, nil
}
