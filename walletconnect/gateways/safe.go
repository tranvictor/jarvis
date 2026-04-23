package gateways

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/util/account"
	"github.com/tranvictor/jarvis/walletconnect"
)

// SafeGateway routes dApp requests through a Gnosis Safe multisig.
//
// Only eth_sendTransaction is supported in v1: the dApp's call is
// wrapped in a SafeTx, submitted to the Safe Transaction Service with
// the local owner's EIP-712 signature, and the safeTxHash is
// returned to the dApp in place of an L1 tx hash. Other owners use
// `jarvis msig approve <safe> <safeTxHash>` to co-sign and execute.
//
// Raw signing methods (personal_sign, eth_signTypedData_v4) are
// rejected with ErrMethodNotSupported — a multisig has no single
// EOA to produce such a signature on behalf of. Some ecosystem
// signatures do exist (EIP-1271 smart-contract signatures), but they
// require an on-chain deployment step that's outside v1's scope.
//
// Chain mobility: the Safe address may or may not have matching code
// on chains other than the one the session opened on. To keep the
// scope small we pin one Safe session to one chain; SwitchChain
// returns ErrChainNotSupported. Users who want to work on two Safes
// on two chains simultaneously should open two jarvis sessions.
type SafeGateway struct {
	ui        walletconnect.UI
	owner     jtypes.AccDesc
	addr      ethcommon.Address
	safe      *safe.SafeContract
	network   jarvisnetworks.Network
	collector safe.SignatureCollector
	resolver  cmdutil.ABIResolver

	unlocked *account.Account
}

// NewSafeGateway prepares the gateway for a Safe at safeAddr on net,
// acting on behalf of ownerAcc. ownerAcc must be one of the Safe's
// current owners; we verify that up front so the dApp is never told
// the session succeeded and then silently rejected at submit-time.
func NewSafeGateway(
	ui walletconnect.UI,
	ownerAcc jtypes.AccDesc,
	safeAddr string,
	net jarvisnetworks.Network,
	resolver cmdutil.ABIResolver,
) (*SafeGateway, error) {
	sc, err := safe.NewSafeContract(safeAddr, net)
	if err != nil {
		return nil, fmt.Errorf("initialise Safe contract at %s: %w", safeAddr, err)
	}
	owners, err := sc.Owners()
	if err != nil {
		return nil, fmt.Errorf("read Safe owners: %w", err)
	}
	isOwner := false
	for _, o := range owners {
		if strings.EqualFold(o, ownerAcc.Address) {
			isOwner = true
			break
		}
	}
	if !isOwner {
		return nil, fmt.Errorf(
			"wallet %s is not an owner of Safe %s", ownerAcc.Address, safeAddr)
	}

	// We prefer a Safe Transaction Service collector; if the chain
	// isn't supported by any STS, we refuse to open the session.
	// v1 WC integration assumes off-chain signature aggregation —
	// on-chain approveHash flows would require every owner to open
	// their own WC session per approval, which is non-starter UX.
	coll, err := safe.NewTxServiceCollector(net.GetChainID())
	if err != nil {
		return nil, fmt.Errorf(
			"no Safe Transaction Service for chain %d: %w",
			net.GetChainID(), err)
	}

	return &SafeGateway{
		ui:        ui,
		owner:     ownerAcc,
		addr:      ethcommon.HexToAddress(safeAddr),
		safe:      sc,
		network:   net,
		collector: coll,
		resolver:  resolver,
	}, nil
}

func (g *SafeGateway) Kind() string    { return "safe" }
func (g *SafeGateway) Account() string {
	return walletconnect.AccountString(g.network.GetChainID(), g.addr.Hex())
}

func (g *SafeGateway) Chains(ctx context.Context) ([]string, error) {
	// A Safe is bound to the chain it was deployed on. We could in
	// theory test for the same address on other chains, but a
	// collision (the Safe deployed at the same address via CREATE2
	// on multiple chains) is common enough that the UX surprise
	// isn't worth it in v1.
	return []string{walletconnect.ChainString(g.network.GetChainID())}, nil
}

func (g *SafeGateway) Methods() []string {
	return []string{
		walletconnect.SupportedMethods.SendTransaction,
	}
}

func (g *SafeGateway) SwitchChain(ctx context.Context, chain string) error {
	return fmt.Errorf(
		"%w: Safe sessions are pinned to the chain they opened on",
		walletconnect.ErrChainNotSupported)
}

func (g *SafeGateway) PersonalSign(ctx context.Context, chain string, message []byte) (string, error) {
	return "", fmt.Errorf("%w: a Safe cannot produce an EOA personal_sign signature",
		walletconnect.ErrMethodNotSupported)
}

func (g *SafeGateway) SignTypedData(ctx context.Context, chain string, typedDataJSON []byte) (string, error) {
	return "", fmt.Errorf("%w: a Safe cannot produce an EOA typed-data signature",
		walletconnect.ErrMethodNotSupported)
}

// SendTransaction turns the dApp's eth_sendTransaction payload into a
// SafeTx proposal, signs it with the local owner's key, submits it
// to the Safe Transaction Service, and returns the safeTxHash (as
// the "tx hash" the dApp expects — this is what Safe's own UI does).
func (g *SafeGateway) SendTransaction(
	ctx context.Context, chain string, tx *walletconnect.RawTx,
) (string, error) {
	// Chain is enforced at the session level, but double-check that
	// nothing has drifted since pair-time.
	expected := walletconnect.ChainString(g.network.GetChainID())
	if chain != expected {
		return "", fmt.Errorf("%w: Safe is on %s, dApp asked for %s",
			walletconnect.ErrChainNotSupported, expected, chain)
	}

	if tx.To == "" {
		return "", fmt.Errorf(
			"refusing eth_sendTransaction with no `to` (Safes can't deploy in v1)")
	}

	value := bigOrZero(tx.Value)
	data := tx.Data
	to := ethcommon.HexToAddress(tx.To)

	nonce, err := nextSafeNonce(g.safe, g.collector)
	if err != nil {
		return "", fmt.Errorf("compute next Safe nonce: %w", err)
	}
	domainSep, err := g.safe.DomainSeparator()
	if err != nil {
		return "", fmt.Errorf("read Safe domainSeparator: %w", err)
	}

	stx := safe.NewSafeTx(to, value, data, safe.OpCall, nonce)
	hash := stx.SafeTxHash(domainSep)

	g.ui.Info("Safe       : %s", shortLabel(g.addr.Hex(), g.network))
	g.ui.Info("Owner      : %s", g.owner.Address)
	g.ui.Info("To         : %s", shortLabel(to.Hex(), g.network))
	if value.Sign() > 0 {
		g.ui.Info("Value      : %s %s",
			jarviscommon.BigToFloat(value, g.network.GetNativeTokenDecimal()),
			g.network.GetNativeTokenSymbol())
	}
	g.ui.Info("Nonce      : %d", nonce)
	if len(data) > 0 {
		preview := ethcommon.Bytes2Hex(data)
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		g.ui.Info("Data       : 0x%s", preview)
	}
	g.ui.Info("safeTxHash : 0x%s", hex.EncodeToString(hash[:]))

	if !g.ui.Confirm("Sign this Safe proposal and submit to the transaction service?", true) {
		return "", walletconnect.ErrUserRejected
	}

	ac, err := g.unlock()
	if err != nil {
		return "", err
	}
	structHash := stx.StructHash()
	sig, err := ac.SignSafeHash(domainSep, structHash)
	if err != nil {
		return "", fmt.Errorf("sign safeTxHash: %w", err)
	}

	if err := g.collector.Propose(
		g.addr, stx, hash,
		ethcommon.HexToAddress(g.owner.Address),
		sig,
	); err != nil {
		return "", fmt.Errorf("propose to Safe Transaction Service: %w", err)
	}

	g.ui.Success("Safe proposal submitted.")
	g.ui.Info("Other owners approve with:")
	g.ui.Info("  jarvis msig approve %s 0x%s", g.addr.Hex(), hex.EncodeToString(hash[:]))

	// Return the safeTxHash. The dApp will try to show it as a tx
	// explorer link; that link won't resolve until the Safe executes,
	// but giving the dApp something well-formed keeps its UI happy.
	return "0x" + hex.EncodeToString(hash[:]), nil
}

// nextSafeNonce mirrors cmd/safe.go's private helper. Duplicated here
// (rather than exported from cmd) because cmd → walletconnect is
// the natural dependency direction and we don't want to invert it.
func nextSafeNonce(sc *safe.SafeContract, c safe.SignatureCollector) (uint64, error) {
	onchain, err := sc.Nonce()
	if err != nil {
		return 0, fmt.Errorf("reading on-chain nonce: %w", err)
	}
	if c == nil {
		return onchain, nil
	}
	next := onchain
	for i := uint64(0); i < 64; i++ {
		pending, err := c.FindByNonce(ethcommon.HexToAddress(sc.Address), next)
		if err != nil {
			if i == 0 {
				return 0, fmt.Errorf("checking pending queue at nonce %d: %w", next, err)
			}
			break
		}
		if pending == nil {
			return next, nil
		}
		next++
	}
	return next, nil
}

func (g *SafeGateway) unlock() (*account.Account, error) {
	if g.unlocked != nil {
		return g.unlocked, nil
	}
	g.ui.Info("Unlocking owner wallet %s for Safe signing…", g.owner.Address)
	ac, err := accounts.UnlockAccount(g.owner)
	if err != nil {
		return nil, fmt.Errorf("unlock wallet: %w", err)
	}
	g.unlocked = ac
	return ac, nil
}
