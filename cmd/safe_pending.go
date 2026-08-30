package cmd

// Pending Safe tx load, hash check, sign, and persist helpers.

import (
	"errors"
	"fmt"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/util"
)

var (
	errPendingNoCollector  = errors.New("safe transaction service is not available")
	errPendingNeedHash     = errors.New("on-chain approve without a service requires an explicit safeTxHash")
	errPendingHashMismatch = errors.New("safeTxHash mismatch")
	errSignSafeHash        = errors.New("sign safeTxHash")
)

// verifyPendingSafeTxHash recomputes the EIP-712 hash when a SafeTx body
// is present. Hash-only --approve-onchain stubs skip the check (nil error).
func verifyPendingSafeTxHash(pending *safe.PendingTx, domainSep [32]byte) ([32]byte, error) {
	var expected [32]byte
	if pending == nil || pending.SafeTx == nil {
		return expected, nil
	}
	expected = pending.SafeTx.SafeTxHash(domainSep)
	if expected != pending.SafeTxHash {
		return expected, errPendingHashMismatch
	}
	return expected, nil
}

func ownerAlreadySigned(pending *safe.PendingTx, me ethcommon.Address) (onChain bool, found bool) {
	if pending == nil {
		return false, false
	}
	for _, s := range pending.Sigs {
		if s.Owner == me {
			return safe.IsOnChainApproval(s.Sig), true
		}
	}
	return false, false
}

func signSafeTx(fromAcc jtypes.AccDesc, stx *safe.SafeTx, domainSep [32]byte) ([]byte, error) {
	account, err := accounts.UnlockAccount(fromAcc)
	if err != nil {
		return nil, err
	}
	sig, err := account.SignSafeHash(domainSep, stx.StructHash())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSignSafeHash, err)
	}
	return sig, nil
}

func signPendingSafeTx(fromAcc jtypes.AccDesc, pending *safe.PendingTx, domainSep [32]byte) ([]byte, error) {
	return signSafeTx(fromAcc, pending.SafeTx, domainSep)
}

func signCause(err error) string {
	prefix := errSignSafeHash.Error() + ": "
	if strings.HasPrefix(err.Error(), prefix) {
		return strings.TrimPrefix(err.Error(), prefix)
	}
	return err.Error()
}

func persistApproval(
	tc cmdutil.TxContext,
	safeAddr string,
	pending *safe.PendingTx,
	me ethcommon.Address,
	sig []byte,
	file string,
	chainID uint64,
) error {
	if file != "" {
		updated := append(append([]safe.OwnerSig{}, pending.Sigs...), safe.OwnerSig{Owner: me, Sig: sig})
		return safe.WriteTxFile(file, safeAddr, chainID, pending.SafeTx, pending.SafeTxHash, updated)
	}
	return tc.Collector.Confirm(pending.SafeTxHash, me, sig)
}

func pendingWithNewSig(pending *safe.PendingTx, me ethcommon.Address, sig []byte) *safe.PendingTx {
	augmented := *pending
	augmented.Sigs = append(append([]safe.OwnerSig{}, pending.Sigs...), safe.OwnerSig{Owner: me, Sig: sig})
	return &augmented
}

// loadPendingTx is the shared pending-tx source selector for approve /
// execute / info: local file, Safe Transaction Service, or (approve only)
// a hash-only stub when --approve-onchain is used without a service.
func loadPendingTx(tc cmdutil.TxContext, args []string, file string, allowOnChainHash bool) (*safe.PendingTx, error) {
	if file != "" {
		_, pending, err := loadPendingFromTxFile(tc, file)
		return pending, err
	}
	if tc.Collector == nil {
		if allowOnChainHash {
			return pendingFromOnChainHash(tc, args)
		}
		return nil, errPendingNoCollector
	}
	identifier, err := pickPendingTxIdentifier(tc, args)
	if err != nil {
		return nil, err
	}
	return resolvePendingTx(tc, identifier)
}

// pendingFromOnChainHash builds a hash-only PendingTx when there is no
// Transaction Service. The caller must have passed a Safe-app URL with a
// hash or a 32-byte 0x… positional arg.
func pendingFromOnChainHash(tc cmdutil.TxContext, args []string) (*safe.PendingTx, error) {
	if tc.SafeAppRef != nil && tc.SafeAppRef.HasTxHash() {
		var hash [32]byte
		copy(hash[:], tc.SafeAppRef.SafeTxHash[:])
		return &safe.PendingTx{
			Safe:       ethcommon.HexToAddress(tc.Safe.Address),
			SafeTxHash: hash,
		}, nil
	}
	if len(args) >= 2 {
		if hash, ok := parseSafeTxHashArg(args[1]); ok {
			return &safe.PendingTx{
				Safe:       ethcommon.HexToAddress(tc.Safe.Address),
				SafeTxHash: hash,
			}, nil
		}
	}
	return nil, errPendingNeedHash
}

func parseSafeTxHashArg(s string) ([32]byte, bool) {
	s = strings.TrimSpace(s)
	var hash [32]byte
	if !strings.HasPrefix(strings.ToLower(s), "0x") || len(s) != 66 {
		return hash, false
	}
	copy(hash[:], ethcommon.FromHex(s))
	return hash, true
}

// loadPendingFromTxFile reads safeTxFile, cross-checks that it matches
// the Safe and chain the user is currently targeting, and returns a
// PendingTx in the same shape the Safe Transaction Service would. It is
// the --safe-tx-file counterpart to pickPendingTxIdentifier +
// resolvePendingTx, used when no service is available or when the user
// deliberately chose file-based signing. Returns the file handle too so
// callers that need to append a signature (approve, off-chain) can write
// back to it without re-reading.
func loadPendingFromTxFile(tc cmdutil.TxContext, path string) (*safe.TxFile, *safe.PendingTx, error) {
	tf, err := safe.ReadTxFile(path)
	if err != nil {
		return nil, nil, err
	}
	if tf.ChainID != config.Network().GetChainID() {
		return nil, nil, fmt.Errorf(
			"tx file %s was built for chain %d but current --network is chain %d",
			path, tf.ChainID, config.Network().GetChainID(),
		)
	}
	if !strings.EqualFold(tf.Safe, tc.Safe.Address) {
		return nil, nil, fmt.Errorf(
			"tx file %s is for safe %s, but this command targets %s",
			path, tf.Safe, tc.Safe.Address,
		)
	}
	pending, err := tf.ToPending()
	if err != nil {
		return nil, nil, fmt.Errorf("decode tx file: %w", err)
	}
	return tf, pending, nil
}

// pickPendingTxIdentifier returns the safeTxHash / nonce string that
// resolvePendingTx should look up. Precedence:
//
//  1. A safeTxHash carried in a Safe-app URL (tc.SafeAppRef.SafeTxHash).
//  2. The second positional argument, if present.
//  3. Auto-pick when the Safe Transaction Service has exactly one pending
//     tx for this Safe (mirrors `jarvis msig` UX).
//
// When several pending txs exist, the function prints a numbered menu and
// errors with an actionable message instead of guessing.
func pickPendingTxIdentifier(tc cmdutil.TxContext, args []string) (string, error) {
	if tc.SafeAppRef != nil && tc.SafeAppRef.HasTxHash() {
		return tc.SafeAppRef.SafeTxHashHex(), nil
	}
	if len(args) >= 2 {
		return args[1], nil
	}
	pending, err := tc.Collector.ListPending(ethcommon.HexToAddress(tc.Safe.Address))
	if err != nil {
		return "", fmt.Errorf(
			"no pending tx identifier given and listing the Safe Transaction Service queue failed: %w",
			err,
		)
	}
	switch len(pending) {
	case 0:
		return "", fmt.Errorf(
			"no pending Safe transactions found for %s. Initiate one with `jarvis msig init`, or pass a safeTxHash / nonce explicitly.",
			tc.Safe.Address,
		)
	case 1:
		p := pending[0]
		appUI.Info(
			"Auto-selected the only pending Safe tx (nonce %s, safeTxHash 0x%s).",
			p.SafeTx.Nonce.String(), ethcommon.Bytes2Hex(p.SafeTxHash[:]),
		)
		return "0x" + ethcommon.Bytes2Hex(p.SafeTxHash[:]), nil
	default:
		appUI.Warn("There are %d pending Safe transactions for %s:", len(pending), tc.Safe.Address)
		for i, p := range pending {
			appUI.Info(
				"  %d. nonce %s  to %s  safeTxHash 0x%s  sigs %d",
				i+1, p.SafeTx.Nonce.String(), p.SafeTx.To.Hex(),
				ethcommon.Bytes2Hex(p.SafeTxHash[:]), len(p.Sigs),
			)
		}
		return "", fmt.Errorf(
			"please specify which one by safeTxHash, nonce, or full Safe-app URL",
		)
	}
}

// resolvePendingTx looks up a pending tx by either safeTxHash (32-byte hex)
// or SafeTx nonce (decimal integer).
func resolvePendingTx(tc cmdutil.TxContext, identifier string) (*safe.PendingTx, error) {
	identifier = strings.TrimSpace(identifier)
	if hash, ok := parseSafeTxHashArg(identifier); ok {
		pt, err := tc.Collector.Get(hash)
		if err != nil {
			return nil, fmt.Errorf("fetching tx 0x%s from service: %w", ethcommon.Bytes2Hex(hash[:]), err)
		}
		return pt, nil
	}
	nonce, err := util.ParamToBigInt(identifier)
	if err != nil {
		return nil, fmt.Errorf("can't interpret %q as a safeTxHash or a nonce", identifier)
	}
	if !nonce.IsUint64() {
		return nil, fmt.Errorf("nonce %s is out of range", identifier)
	}
	pt, err := tc.Collector.FindByNonce(
		ethcommon.HexToAddress(tc.Safe.Address), nonce.Uint64(),
	)
	if err != nil {
		return nil, fmt.Errorf("looking up nonce %s in service queue: %w", identifier, err)
	}
	if pt == nil {
		return nil, fmt.Errorf("no pending Safe transaction at nonce %s", identifier)
	}
	return pt, nil
}
