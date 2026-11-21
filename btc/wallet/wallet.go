// Package wallet stores Bitcoin wallet records and scans them for coins.
//
// Records live in ~/.jarvis/btc/, one JSON file per wallet. They are kept in
// a subdirectory on purpose: the Ethereum side of Jarvis globs
// ~/.jarvis/*.json for wallets and must never see a Bitcoin record.
package wallet

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/base58"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"

	"github.com/tranvictor/jarvis/btc"
)

const (
	KindKeystore = "keystore"
	KindTrezor   = "trezor"
	KindLedger   = "ledger"
)

// Record is what is persisted for a wallet. It never holds a secret: the
// keystore kind points at an encrypted file, hardware kinds hold only the
// account-level extended public key.
type Record struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Chain string `json:"chain"`
	Desc  string `json:"desc,omitempty"`

	// keystore: one P2WPKH address for one key.
	Keystore string `json:"keystore,omitempty"`
	Address  string `json:"address,omitempty"`
	PubKey   string `json:"pubkey,omitempty"`

	// trezor / ledger: BIP-84 account.
	AccountPath string `json:"account_path,omitempty"`
	XPub        string `json:"xpub,omitempty"`
	// MasterFP is the BIP-32 master fingerprint (hex). Ledger needs it to
	// name the signing key inside a PSBT; Trezor does not report it.
	MasterFP string `json:"master_fingerprint,omitempty"`
}

func (r Record) IsHD() bool { return r.Kind == KindTrezor || r.Kind == KindLedger }

func (r Record) ChainParsed() (btc.Chain, error) { return btc.ParseChain(r.Chain) }

// Derived is one wallet address together with everything a signer needs.
type Derived struct {
	Address string
	Script  []byte
	PubKey  []byte
	// Branch is 0 for receive and 1 for change; Index is the address index.
	// Both are meaningless for keystore wallets, which have a single key.
	Branch uint32
	Index  uint32
}

type Wallet struct {
	Rec    Record
	Chain  btc.Chain
	Params *chaincfg.Params
	acct   *hdkeychain.ExtendedKey
	single *btcec.PublicKey
}

func Open(rec Record) (*Wallet, error) {
	chain, err := rec.ChainParsed()
	if err != nil {
		return nil, err
	}
	w := &Wallet{Rec: rec, Chain: chain, Params: chain.Params()}
	switch rec.Kind {
	case KindKeystore:
		raw, err := hex.DecodeString(rec.PubKey)
		if err != nil {
			return nil, fmt.Errorf("wallet %q: bad public key: %w", rec.Name, err)
		}
		pk, err := btcec.ParsePubKey(raw)
		if err != nil {
			return nil, fmt.Errorf("wallet %q: bad public key: %w", rec.Name, err)
		}
		w.single = pk
		d, err := w.Derive(0, 0)
		if err != nil {
			return nil, err
		}
		if rec.Address != "" && !strings.EqualFold(rec.Address, d.Address) {
			return nil, fmt.Errorf("wallet %q: stored address %s does not match its public key (%s); the record is corrupt or for another chain", rec.Name, rec.Address, d.Address)
		}
	case KindTrezor, KindLedger:
		acct, err := ParseXPub(rec.XPub, w.Params)
		if err != nil {
			return nil, fmt.Errorf("wallet %q: %w", rec.Name, err)
		}
		w.acct = acct
	default:
		return nil, fmt.Errorf("wallet %q has unsupported kind %q", rec.Name, rec.Kind)
	}
	return w, nil
}

// ParseXPub accepts an extended public key with any SLIP-132 version
// (xpub, zpub, tpub, vpub, ...) and returns it in the version the chain's
// params use, so btcd's key derivation accepts it.
func ParseXPub(s string, params *chaincfg.Params) (*hdkeychain.ExtendedKey, error) {
	raw := base58.Decode(strings.TrimSpace(s))
	if len(raw) != 82 {
		return nil, errors.New("invalid extended public key")
	}
	body, sum := raw[:78], raw[78:]
	if !bytes.Equal(chainhash.DoubleHashB(body)[:4], sum) {
		return nil, errors.New("invalid extended public key: bad checksum")
	}
	copy(body[:4], params.HDPublicKeyID[:])
	fixed := append(append([]byte{}, body...), chainhash.DoubleHashB(body)[:4]...)
	key, err := hdkeychain.NewKeyFromString(base58.Encode(fixed))
	if err != nil {
		return nil, fmt.Errorf("invalid extended public key: %w", err)
	}
	if key.IsPrivate() {
		return nil, errors.New("refusing an extended private key: wallets store public keys only")
	}
	return key, nil
}

// AccountPath is the BIP-32 path of the account key an HD wallet was
// created from.
func (w *Wallet) AccountPath() ([]uint32, error) {
	return btc.ParsePath(w.Rec.AccountPath)
}

// AccountNode returns the compressed public key and chain code of the
// account key, which is how a device's answer is compared with the record.
func (w *Wallet) AccountNode() (pub, chainCode []byte, err error) {
	if w.acct == nil {
		return nil, nil, fmt.Errorf("wallet %q has no account key", w.Rec.Name)
	}
	pk, err := w.acct.ECPubKey()
	if err != nil {
		return nil, nil, err
	}
	return pk.SerializeCompressed(), w.acct.ChainCode(), nil
}

func (w *Wallet) Derive(branch, index uint32) (Derived, error) {
	var pub *btcec.PublicKey
	if w.single != nil {
		pub = w.single
		branch, index = 0, 0
	} else {
		if branch > 1 {
			return Derived{}, fmt.Errorf("branch %d is not receive (0) or change (1)", branch)
		}
		b, err := w.acct.Derive(branch)
		if err != nil {
			return Derived{}, err
		}
		k, err := b.Derive(index)
		if err != nil {
			return Derived{}, err
		}
		pub, err = k.ECPubKey()
		if err != nil {
			return Derived{}, err
		}
	}
	compressed := pub.SerializeCompressed()
	addr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(compressed), w.Params)
	if err != nil {
		return Derived{}, err
	}
	script, err := btc.PayTo(addr)
	if err != nil {
		return Derived{}, err
	}
	return Derived{Address: addr.EncodeAddress(), Script: script, PubKey: compressed, Branch: branch, Index: index}, nil
}
