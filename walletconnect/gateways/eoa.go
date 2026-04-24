package gateways

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	jarvisui "github.com/tranvictor/jarvis/ui"
	jarvisutil "github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/account"
	"github.com/tranvictor/jarvis/util/broadcaster"
	utilreader "github.com/tranvictor/jarvis/util/reader"
	"github.com/tranvictor/jarvis/walletconnect"
)

// EOAGateway services dApp requests with a local (or hardware-backed)
// externally-owned account. One gateway serves one session; chain
// switches rebind the reader/broadcaster but keep the unlocked
// account.
//
// Concurrency: the session layer dispatches synchronously, so we only
// need a mutex to protect chain-switch rebinds.
type EOAGateway struct {
	ui   walletconnect.UI
	acc  jtypes.AccDesc
	addr ethcommon.Address

	// unlocked is a cached Account from accounts.UnlockAccount —
	// constructed lazily on first sign to avoid prompting for a
	// passphrase during the pair-time proposal step (where the user
	// might still reject).
	unlockMu sync.Mutex
	unlocked *account.Account

	// chainMu guards cur* fields across SwitchChain.
	chainMu sync.RWMutex
	curNet  jarvisnetworks.Network
	curRD   utilreader.Reader
	curBC   *broadcaster.Broadcaster

	// resolver / analyzer are used only for the transaction confirm
	// prompt (decoding calldata, looking up contract labels).
	resolver cmdutil.ABIResolver
}

// NewEOAGateway prepares a gateway bound to the given local account.
// defaultNetwork is the chain the session opens on; the gateway will
// advertise all jarvis-known eip155 chains and rebind on a
// wallet_switchEthereumChain request.
func NewEOAGateway(
	ui walletconnect.UI,
	acc jtypes.AccDesc,
	defaultNetwork jarvisnetworks.Network,
	resolver cmdutil.ABIResolver,
) (*EOAGateway, error) {
	g := &EOAGateway{
		ui:       ui,
		acc:      acc,
		addr:     ethcommon.HexToAddress(acc.Address),
		resolver: resolver,
	}
	if err := g.bindChain(defaultNetwork); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *EOAGateway) Kind() string    { return "eoa" }
func (g *EOAGateway) Account() string {
	// CAIP-10 for the current chain; the session layer asks for this
	// at settle-time so the chain here matters.
	g.chainMu.RLock()
	defer g.chainMu.RUnlock()
	return walletconnect.AccountString(g.curNet.GetChainID(), g.addr.Hex())
}

func (g *EOAGateway) Chains(ctx context.Context) ([]string, error) {
	// An EOA can operate on every chain jarvis knows — no contract
	// check is meaningful for private keys.
	return allKnownEip155Chains(), nil
}

func (g *EOAGateway) Methods() []string {
	return []string{
		walletconnect.SupportedMethods.SendTransaction,
		walletconnect.SupportedMethods.PersonalSign,
		walletconnect.SupportedMethods.SignTypedDataV4,
		walletconnect.SupportedMethods.SwitchChain,
	}
}

// SwitchChain rebinds the gateway's reader/broadcaster to the target
// chain. The account's address is the same on every EIP-155 chain so
// nothing about the unlocked key changes.
func (g *EOAGateway) SwitchChain(ctx context.Context, chain string) error {
	net, err := networkForChain(chain)
	if err != nil {
		return err
	}
	return g.bindChain(net)
}

func (g *EOAGateway) bindChain(net jarvisnetworks.Network) error {
	rd, err := jarvisNetReader(net)
	if err != nil {
		return fmt.Errorf("no reader for %s: %w", net.GetName(), err)
	}
	bc, err := jarvisNetBroadcaster(net)
	if err != nil {
		return fmt.Errorf("no broadcaster for %s: %w", net.GetName(), err)
	}
	g.chainMu.Lock()
	g.curNet = net
	g.curRD = rd
	g.curBC = bc
	g.chainMu.Unlock()
	return nil
}

// SendTransaction builds a regular EOA tx from the dApp payload,
// fills in missing nonce/gas/fee fields from the current node,
// prompts the operator with decoded calldata, signs, broadcasts and
// returns the tx hash.
//
// The chain argument is authoritative: the dApp must address its
// request to a chain that's in the session's eip155 namespace, which
// we've already validated at the session layer. Here we just verify
// that the current bound chain matches what the dApp asked for (it
// might not if a switch just happened in the other direction).
func (g *EOAGateway) SendTransaction(
	ctx context.Context, chain string, tx *walletconnect.RawTx,
) (string, error) {
	net, err := networkForChain(chain)
	if err != nil {
		return "", err
	}
	g.chainMu.Lock()
	if g.curNet.GetChainID() != net.GetChainID() {
		if err := g.bindChainLocked(net); err != nil {
			g.chainMu.Unlock()
			return "", err
		}
	}
	rd := g.curRD
	bc := g.curBC
	g.chainMu.Unlock()

	if err := ensureSameAddress(tx.From, g.addr.Hex()); err != nil {
		return "", err
	}

	txType, err := cmdutil.ValidTxType(rd, net)
	if err != nil {
		return "", fmt.Errorf("pick tx type: %w", err)
	}

	nonce := tx.Nonce
	if !tx.NonceProvided {
		nonce, err = rd.GetMinedNonce(g.addr.Hex())
		if err != nil {
			return "", fmt.Errorf("get nonce: %w", err)
		}
	}

	value := bigOrZero(tx.Value)
	data := tx.Data

	gasLimit := tx.Gas
	if gasLimit == 0 {
		est, err := rd.EstimateExactGas(g.addr.Hex(), tx.To, 0, value, data)
		if err != nil {
			return "", fmt.Errorf("estimate gas: %w", err)
		}
		gasLimit = est + 50_000 // modest buffer; dApps rarely send tight estimates.
	}

	// Resolve gas pricing. If the dApp provided EIP-1559 fields and
	// we're on a 1559-capable chain, honour them; otherwise derive
	// from the node's recommendation.
	priceGwei := 0.0
	tipGwei := 0.0
	if txType == types.DynamicFeeTxType {
		if tx.MaxFeePerGas != nil {
			priceGwei = jarviscommon.BigToFloat(tx.MaxFeePerGas, 9)
		} else {
			priceGwei, err = rd.RecommendedGasPrice()
			if err != nil {
				return "", fmt.Errorf("recommended gas price: %w", err)
			}
		}
		if tx.MaxPriorityFeePerGas != nil {
			tipGwei = jarviscommon.BigToFloat(tx.MaxPriorityFeePerGas, 9)
		} else {
			tip, err := rd.GetSuggestedGasTipCap()
			if err == nil {
				tipGwei = tip
			}
		}
	} else {
		if tx.GasPrice != nil {
			priceGwei = jarviscommon.BigToFloat(tx.GasPrice, 9)
		} else {
			priceGwei, err = rd.RecommendedGasPrice()
			if err != nil {
				return "", fmt.Errorf("recommended gas price: %w", err)
			}
		}
	}

	signedTxTo := tx.To
	if signedTxTo == "" {
		// Contract creation: BuildExactTx expects a string address;
		// go-ethereum treats zero-address differently for
		// types.Transaction, so we bail out for now — dApp-driven
		// contract deploys are rare and dangerous enough to refuse.
		return "", fmt.Errorf(
			"refusing eth_sendTransaction with no `to` (contract creation)")
	}

	ethTx := jarviscommon.BuildExactTx(
		txType, nonce, signedTxTo, value, gasLimit, priceGwei, tipGwei, data,
		net.GetChainID(),
	)

	// Delegate the confirmation to the standard jarvis prompt so the
	// operator sees the same rich decoded view (token transfers, ENS
	// labels, param-by-param breakdown) they'd get from `jarvis send`,
	// and gets exactly one confirm prompt. Falls back to the minimal
	// inline summary if the UI doesn't satisfy the full ui.UI
	// interface (e.g. a trimmed test fake).
	if err := g.promptSignConfirm(net, rd, ethTx, signedTxTo, data); err != nil {
		return "", err
	}

	ac, err := g.unlock()
	if err != nil {
		return "", err
	}
	signer, signedTx, err := ac.SignTx(ethTx, big.NewInt(int64(net.GetChainID())))
	if err != nil {
		return "", fmt.Errorf("sign tx: %w", err)
	}
	if signer.Cmp(g.addr) != 0 {
		return "", fmt.Errorf(
			"signed as %s but session is %s (wrong wallet / HW?)",
			signer.Hex(), g.addr.Hex())
	}

	hash, ok, err := bc.BroadcastTx(signedTx)
	if !ok {
		return "", fmt.Errorf("broadcast rejected: %w", err)
	}
	g.ui.Success("Broadcasted. Tx hash: %s", hash)

	// Wait for the tx to be mined and print the full jarvis receipt
	// analysis (decoded logs, ERC20 transfers, …) — same output as
	// `jarvis send` gives you. Runs in a goroutine so the dApp gets
	// its eth_sendTransaction response back immediately; if the user
	// Ctrl-Cs the session before the tx mines, the goroutine dies
	// with the process.
	if fullUI, ok := g.ui.(jarvisui.UI); ok {
		go g.waitAndAnalyze(fullUI, rd, net, signedTx, signedTxTo)
	}
	return hash, nil
}

// waitAndAnalyze blocks until signedTx is mined, then prints the
// jarvis-standard receipt summary using the full ui.UI. Runs in a
// goroutine so the dApp's eth_sendTransaction response isn't
// gated on block confirmation. On session teardown the goroutine
// dies with the process.
func (g *EOAGateway) waitAndAnalyze(
	fullUI jarvisui.UI,
	rd utilreader.Reader,
	net jarvisnetworks.Network,
	signedTx *types.Transaction,
	to string,
) {
	analyzer := txanalyzer.NewGenericAnalyzer(rd, net)
	var customABIs map[string]*abi.ABI
	if a, err := g.resolver.ConfigToABI(to, false, "", net); err == nil && a != nil {
		customABIs = map[string]*abi.ABI{strings.ToLower(to): a}
	}
	jarvisutil.DisplayWaitAnalyze(
		fullUI, rd, analyzer, signedTx, true, nil, net,
		nil, customABIs, config.DegenMode,
	)
}

func (g *EOAGateway) PersonalSign(ctx context.Context, chain string, message []byte) (string, error) {
	g.ui.Info("Message (%d bytes):", len(message))
	g.ui.Info("  %s", displayableMessage(message))
	if !g.ui.Confirm("Sign this message?", true) {
		return "", walletconnect.ErrUserRejected
	}
	ac, err := g.unlock()
	if err != nil {
		return "", err
	}
	sig, err := ac.SignPersonalMessage(message)
	if err != nil {
		return "", fmt.Errorf("personal_sign: %w", err)
	}
	return hex0x(sig), nil
}

func (g *EOAGateway) SignTypedData(ctx context.Context, chain string, typedDataJSON []byte) (string, error) {
	td, err := account.ParseTypedDataV4(typedDataJSON)
	if err != nil {
		return "", err
	}
	g.ui.Info("Domain    : %s (chainId %v)",
		td.Domain.Name,
		td.Domain.ChainId,
	)
	if td.Domain.VerifyingContract != "" {
		g.ui.Info("Verifying : %s", td.Domain.VerifyingContract)
	}
	g.ui.Info("PrimaryType: %s", td.PrimaryType)
	g.ui.Info("Message   : %s", firstLineOf(string(typedDataJSON), 200))
	if !g.ui.Confirm("Sign this typed-data message?", true) {
		return "", walletconnect.ErrUserRejected
	}
	ac, err := g.unlock()
	if err != nil {
		return "", err
	}
	sig, err := ac.SignTypedDataV4(td)
	if err != nil {
		return "", fmt.Errorf("eth_signTypedData_v4: %w", err)
	}
	return hex0x(sig), nil
}

// bindChainLocked assumes the caller holds g.chainMu.
func (g *EOAGateway) bindChainLocked(net jarvisnetworks.Network) error {
	rd, err := jarvisNetReader(net)
	if err != nil {
		return fmt.Errorf("no reader for %s: %w", net.GetName(), err)
	}
	bc, err := jarvisNetBroadcaster(net)
	if err != nil {
		return fmt.Errorf("no broadcaster for %s: %w", net.GetName(), err)
	}
	g.curNet = net
	g.curRD = rd
	g.curBC = bc
	return nil
}

// unlock returns the unlocked account, prompting for passphrase on
// first call. HW wallets reuse the same Account across operations;
// device-level confirmation still happens per-sign.
func (g *EOAGateway) unlock() (*account.Account, error) {
	g.unlockMu.Lock()
	defer g.unlockMu.Unlock()
	if g.unlocked != nil {
		return g.unlocked, nil
	}
	g.ui.Info("Unlocking wallet %s for signing…", g.addr.Hex())
	ac, err := accounts.UnlockAccount(g.acc)
	if err != nil {
		return nil, fmt.Errorf("unlock wallet: %w", err)
	}
	g.unlocked = ac
	return ac, nil
}

// promptSignConfirm shows the transaction to the operator using the
// standard jarvis PromptTxConfirmation flow (same rich decoded view +
// single "Confirm?" prompt as `jarvis send`) and returns
// walletconnect.ErrUserRejected if the user bails. The custom inline
// summary we used to print before this prompt has been removed — it
// was just a degraded copy of what PromptTxConfirmation already shows.
//
// If the injected UI is not a full ui.UI (the WalletConnect interface
// is intentionally a subset), we fall back to a compact inline
// summary + a yes/no prompt so tests and alt UIs still get the
// confirmation step.
func (g *EOAGateway) promptSignConfirm(
	net jarvisnetworks.Network,
	rd utilreader.Reader,
	tx *types.Transaction,
	to string,
	data []byte,
) error {
	if fullUI, ok := g.ui.(jarvisui.UI); ok {
		// util.NewEnrichedResolver (used by both util.GetJarvisAddress
		// and txanalyzer.NewGenericAnalyzer) automatically prefetches
		// verified contract names from the block explorer on first
		// miss, so we don't have to remember to call
		// PrefetchContractName per address here or downstream.
		analyzer := txanalyzer.NewGenericAnalyzer(rd, net)
		var customABIs map[string]*abi.ABI
		if a, err := g.resolver.ConfigToABI(to, false, "", net); err == nil && a != nil {
			customABIs = map[string]*abi.ABI{
				strings.ToLower(to): a,
			}
		}
		fromAddr := jarvisutil.GetJarvisAddress(g.addr.Hex(), net)
		if err := cmdutil.PromptTxConfirmation(
			fullUI, analyzer, fromAddr, tx, customABIs, net,
		); err != nil {
			return walletconnect.ErrUserRejected
		}
		return nil
	}
	return g.fallbackConfirm(net, tx, to, data)
}

// fallbackConfirm is the pre-PromptTxConfirmation summary + prompt,
// kept as a last-resort code path for UIs that only satisfy the
// walletconnect.UI subset.
func (g *EOAGateway) fallbackConfirm(
	net jarvisnetworks.Network,
	tx *types.Transaction,
	to string,
	data []byte,
) error {
	g.ui.Info("From  : %s", shortLabel(g.addr.Hex(), net))
	g.ui.Info("To    : %s", shortLabel(to, net))
	if v := tx.Value(); v != nil && v.Sign() > 0 {
		g.ui.Info("Value : %s %s (%s wei)",
			strings.TrimRight(strings.TrimRight(
				fmt.Sprintf("%.6f", jarviscommon.BigToFloat(v, net.GetNativeTokenDecimal())),
				"0"), "."),
			net.GetNativeTokenSymbol(), v.String())
	}
	g.ui.Info("Gas   : %d", tx.Gas())
	if len(data) > 0 {
		hexData := ethcommon.Bytes2Hex(data)
		preview := hexData
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		g.ui.Info("Data  : 0x%s", preview)
		if a, err := g.resolver.ConfigToABI(to, false, "", net); err == nil && a != nil {
			if m, ok := matchMethod(a, data); ok {
				g.ui.Info("Call  : %s", m)
			}
		}
	}
	if !g.ui.Confirm("Sign and broadcast this transaction?", true) {
		return walletconnect.ErrUserRejected
	}
	return nil
}

func matchMethod(a *abi.ABI, data []byte) (string, bool) {
	if len(data) < 4 {
		return "", false
	}
	for name, m := range a.Methods {
		if string(m.ID) == string(data[:4]) {
			return name, true
		}
	}
	return "", false
}

// displayableMessage renders a personal_sign message as UTF-8 if it's
// printable ASCII, otherwise as hex. Matches what MetaMask shows.
func displayableMessage(msg []byte) string {
	printable := true
	for _, b := range msg {
		if b < 0x20 || b > 0x7e {
			if b != '\n' && b != '\t' && b != '\r' {
				printable = false
				break
			}
		}
	}
	if printable {
		return string(msg)
	}
	return "0x" + ethcommon.Bytes2Hex(msg)
}

func firstLineOf(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
