package gateways

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	jarvisui "github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/account"
	"github.com/tranvictor/jarvis/util/broadcaster"
	utilreader "github.com/tranvictor/jarvis/util/reader"
	"github.com/tranvictor/jarvis/walletconnect"
)

// ClassicGateway routes dApp requests through a legacy Gnosis
// MultiSigWallet. The dApp's inner call is wrapped in
// submitTransaction(to, value, data), signed by the local owner
// wallet, and broadcast as a plain EOA transaction. The outer tx
// hash is returned — other owners go on to confirm via
// `jarvis msig approve <msig> <txid>`.
//
// Like SafeGateway, raw signing methods are unsupported.
type ClassicGateway struct {
	ui       walletconnect.UI
	owner    jtypes.AccDesc
	addr     ethcommon.Address
	msig     *msig.MultisigContract
	network  jarvisnetworks.Network
	reader   utilreader.Reader
	bc       *broadcaster.Broadcaster
	resolver cmdutil.ABIResolver

	unlocked *account.Account
}

// NewClassicGateway prepares a gateway for the classic multisig at
// msigAddr on net. ownerAcc must be one of the msig's current
// owners.
func NewClassicGateway(
	ui walletconnect.UI,
	ownerAcc jtypes.AccDesc,
	msigAddr string,
	net jarvisnetworks.Network,
	resolver cmdutil.ABIResolver,
) (*ClassicGateway, error) {
	mc, err := msig.NewMultisigContract(msigAddr, net)
	if err != nil {
		return nil, fmt.Errorf("initialise classic multisig at %s: %w", msigAddr, err)
	}
	owners, err := mc.Owners()
	if err != nil {
		return nil, fmt.Errorf("read multisig owners: %w", err)
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
			"wallet %s is not an owner of multisig %s", ownerAcc.Address, msigAddr)
	}

	rd, err := jarvisNetReader(net)
	if err != nil {
		return nil, fmt.Errorf("no reader for %s: %w", net.GetName(), err)
	}
	bc, err := jarvisNetBroadcaster(net)
	if err != nil {
		return nil, fmt.Errorf("no broadcaster for %s: %w", net.GetName(), err)
	}

	return &ClassicGateway{
		ui:       ui,
		owner:    ownerAcc,
		addr:     ethcommon.HexToAddress(msigAddr),
		msig:     mc,
		network:  net,
		reader:   rd,
		bc:       bc,
		resolver: resolver,
	}, nil
}

func (g *ClassicGateway) Kind() string    { return "classic" }
func (g *ClassicGateway) Account() string {
	return walletconnect.AccountString(g.network.GetChainID(), g.addr.Hex())
}

func (g *ClassicGateway) Chains(ctx context.Context) ([]string, error) {
	// Unlike a Safe, a classic msig is just contract code — the same
	// bytecode might live at the same address on other chains. But
	// membership (owners, thresholds, queued txs) does not transfer.
	// Pinning to one chain keeps the UX consistent with Safe sessions.
	return []string{walletconnect.ChainString(g.network.GetChainID())}, nil
}

func (g *ClassicGateway) Methods() []string {
	return []string{walletconnect.SupportedMethods.SendTransaction}
}

// SwitchChain: a classic msig is just contract code, so the address
// could in theory be a valid msig on another chain too — but we'd
// need to re-probe DetectMultisigType against that chain's RPC and
// re-bind the owner set. Since that's strictly more work than the
// user can accomplish by opening a new session, we refuse here too.
func (g *ClassicGateway) SwitchChain(ctx context.Context, chain string) error {
	return fmt.Errorf(
		"%w: classic-multisig sessions are pinned to the chain they opened on",
		walletconnect.ErrChainNotSupported)
}

func (g *ClassicGateway) PersonalSign(ctx context.Context, chain string, message []byte) (string, error) {
	return "", fmt.Errorf("%w: a classic multisig can't produce personal_sign signatures",
		walletconnect.ErrMethodNotSupported)
}

func (g *ClassicGateway) SignTypedData(ctx context.Context, chain string, typedDataJSON []byte) (string, error) {
	return "", fmt.Errorf("%w: a classic multisig can't produce typed-data signatures",
		walletconnect.ErrMethodNotSupported)
}

// SendTransaction wraps the inner call in submitTransaction,
// estimates gas, prompts, signs with the owner wallet, and
// broadcasts. Returns the outer tx hash (not an inner multisig
// txid — dApps expect a hex tx hash here).
func (g *ClassicGateway) SendTransaction(
	ctx context.Context, chain string, tx *walletconnect.RawTx,
) (string, error) {
	expected := walletconnect.ChainString(g.network.GetChainID())
	if chain != expected {
		return "", fmt.Errorf("%w: msig is on %s, dApp asked for %s",
			walletconnect.ErrChainNotSupported, expected, chain)
	}

	if tx.To == "" {
		return "", fmt.Errorf(
			"refusing eth_sendTransaction with no `to` (classic msig cannot deploy)")
	}

	innerValue := bigOrZero(tx.Value)
	innerData := tx.Data
	innerTo := ethcommon.HexToAddress(tx.To)

	// Wrap in submitTransaction(address, uint256, bytes).
	msigABI := util.GetGnosisMsigABI()
	outerData, err := msigABI.Pack("submitTransaction", innerTo, innerValue, innerData)
	if err != nil {
		return "", fmt.Errorf("pack submitTransaction: %w", err)
	}

	ownerAddr := g.owner.Address

	txType, err := cmdutil.ValidTxType(g.reader, g.network)
	if err != nil {
		return "", fmt.Errorf("pick tx type: %w", err)
	}
	nonce, err := g.reader.GetMinedNonce(ownerAddr)
	if err != nil {
		return "", fmt.Errorf("get owner nonce: %w", err)
	}
	gasLimit, err := g.reader.EstimateExactGas(
		ownerAddr, g.addr.Hex(), 0, big.NewInt(0), outerData,
	)
	if err != nil {
		return "", fmt.Errorf("estimate gas: %w", err)
	}
	gasLimit += 60_000

	priceGwei, err := g.reader.RecommendedGasPrice()
	if err != nil {
		return "", fmt.Errorf("recommended gas price: %w", err)
	}
	tipGwei := 0.0
	if txType == types.DynamicFeeTxType {
		if tip, err := g.reader.GetSuggestedGasTipCap(); err == nil {
			tipGwei = tip
		}
	}

	ethTx := jarviscommon.BuildExactTx(
		txType, nonce, g.addr.Hex(), big.NewInt(0), gasLimit,
		priceGwei, tipGwei, outerData, g.network.GetChainID(),
	)

	g.ui.Info("Multisig : %s", shortLabel(g.addr.Hex(), g.network))
	g.ui.Info("Owner    : %s", ownerAddr)
	g.ui.Info("To       : %s", shortLabel(innerTo.Hex(), g.network))
	if innerValue.Sign() > 0 {
		g.ui.Info("Value    : %s %s",
			jarviscommon.BigToFloat(innerValue, g.network.GetNativeTokenDecimal()),
			g.network.GetNativeTokenSymbol())
	}
	if len(innerData) > 0 {
		preview := ethcommon.Bytes2Hex(innerData)
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		g.ui.Info("Data     : 0x%s", preview)
	}
	g.ui.Info("Outer gas: %d @ %v gwei", gasLimit, priceGwei)

	if !g.ui.Confirm(
		"Wrap this call in submitTransaction and broadcast it from the owner wallet?",
		true,
	) {
		return "", walletconnect.ErrUserRejected
	}

	ac, err := g.unlock()
	if err != nil {
		return "", err
	}
	signer, signedTx, err := ac.SignTx(ethTx, big.NewInt(int64(g.network.GetChainID())))
	if err != nil {
		return "", fmt.Errorf("sign outer tx: %w", err)
	}
	if !strings.EqualFold(signer.Hex(), ownerAddr) {
		return "", fmt.Errorf(
			"signed as %s, expected owner %s", signer.Hex(), ownerAddr)
	}
	hash, ok, err := g.bc.BroadcastTx(signedTx)
	if !ok {
		return "", fmt.Errorf("broadcast rejected: %w", err)
	}
	g.ui.Success("Broadcasted submitTransaction. Outer tx hash: %s", hash)
	g.ui.Info(
		"Other owners approve with:  jarvis msig approve %s %s",
		g.addr.Hex(), hash,
	)

	// Same treatment as the EOA gateway: after broadcast, wait for
	// the outer submitTransaction to be mined and print the full
	// jarvis receipt (decoded Submission/Confirmation events, etc.)
	// via the standard analyzer. Runs off the hot path so the dApp
	// gets its eth_sendTransaction response immediately.
	if fullUI, ok := g.ui.(jarvisui.UI); ok {
		go g.waitAndAnalyze(fullUI, signedTx)
	}
	return hash, nil
}

func (g *ClassicGateway) waitAndAnalyze(
	fullUI jarvisui.UI,
	signedTx *types.Transaction,
) {
	analyzer := txanalyzer.NewGenericAnalyzer(g.reader, g.network)
	util.DisplayWaitAnalyze(
		fullUI, g.reader, analyzer, signedTx, true, nil, g.network,
		nil, nil, config.DegenMode,
	)
}

func (g *ClassicGateway) unlock() (*account.Account, error) {
	if g.unlocked != nil {
		return g.unlocked, nil
	}
	g.ui.Info("Unlocking owner wallet %s for signing…", g.owner.Address)
	ac, err := accounts.UnlockAccount(g.owner)
	if err != nil {
		return nil, fmt.Errorf("unlock wallet: %w", err)
	}
	g.unlocked = ac
	return ac, nil
}
