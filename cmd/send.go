package cmd

import (
	"errors"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	types2 "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/util"
	utilreader "github.com/tranvictor/jarvis/util/reader"
)

// flag-bound package-level vars (Cobra writes to these before Run is called).
var (
	to    string
	value string
	data  string
)

// sendTxParams carries all resolved transaction parameters so that helpers do
// not need to read or mutate any config.* globals.
type sendTxParams struct {
	txType   uint8
	nonce    uint64
	gasLimit uint64 // already includes ExtraGasLimit
	gasPrice float64 // already includes ExtraGasPrice
	tipGas   float64 // already includes ExtraTipGas
}

func handleMsigSend(
	sp sendTxParams,
	from types2.AccDesc,
	msigAddr string,
	txdata []byte,
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	bc cmdutil.TxBroadcaster,
) {
	t := jarviscommon.BuildExactTx(
		sp.txType,
		sp.nonce,
		msigAddr,
		big.NewInt(0),
		sp.gasLimit,
		sp.gasPrice,
		sp.tipGas,
		txdata,
		config.Network().GetChainID(),
	)

	if broadcasted, err := cmdutil.SignAndBroadcast(appUI, from, t, nil, reader, analyzer, nil, bc); err != nil && !broadcasted {
		if errors.Is(err, cmdutil.ErrWalletUnlock) {
			os.Exit(126)
		}
		appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

// handleSend builds and broadcasts either a native-token or ERC-20 transfer.
// extraData is the raw hex payload from the --data flag (empty for plain sends).
func handleSend(
	sp sendTxParams,
	from types2.AccDesc,
	toAddr string,
	amountWei *big.Int,
	tokenAddr string,
	extraData string,
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	bc cmdutil.TxBroadcaster,
) {
	var (
		t *types.Transaction
		a *abi.ABI
	)

	if tokenAddr == util.ETH_ADDR {
		t = jarviscommon.BuildExactTx(
			sp.txType,
			sp.nonce,
			toAddr,
			amountWei,
			sp.gasLimit,
			sp.gasPrice,
			sp.tipGas,
			cmdutil.StringParamToBytes(extraData),
			config.Network().GetChainID(),
		)
	} else {
		a = jarviscommon.GetERC20ABI()
		erc20data, err := a.Pack("transfer", jarviscommon.HexToAddress(toAddr), amountWei)
		if err != nil {
			appUI.Error("Couldn't pack data: %s", err)
			return
		}
		t = jarviscommon.BuildExactTx(
			sp.txType,
			sp.nonce,
			tokenAddr,
			big.NewInt(0),
			sp.gasLimit,
			sp.gasPrice,
			sp.tipGas,
			erc20data,
			config.Network().GetChainID(),
		)
	}

	if broadcasted, err := cmdutil.SignAndBroadcast(
		appUI, from, t,
		map[string]*abi.ABI{strings.ToLower(tokenAddr): a},
		reader, analyzer, a, bc,
	); err != nil && !broadcasted {
		if errors.Is(err, cmdutil.ErrWalletUnlock) {
			os.Exit(126)
		}
		appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

func sendFromMsig(reader utilreader.Reader, analyzer util.TxAnalyzer, resolver cmdutil.ABIResolver, bc cmdutil.TxBroadcaster) {
	msigAddress, err := getMsigContractFromParams([]string{config.From}, resolver)
	if err != nil {
		appUI.Error("Couldn't find a wallet or multisig with keyword %s", config.From)
		return
	}

	msigContractAddr, _, err := resolver.GetAddressFromString(msigAddress)
	if err != nil {
		appUI.Error("Couldn't find a wallet or multisig with keyword \"%s\"", config.From)
		return
	}

	multisigContract, err := msig.NewMultisigContract(msigContractAddr, config.Network())
	if err != nil {
		appUI.Error("Couldn't read the multisig: %s", err)
		return
	}

	owners, err := multisigContract.Owners()
	if err != nil {
		appUI.Error("getting msig owners failed: %s", err)
		return
	}

	var fromAcc types2.AccDesc
	for _, owner := range owners {
		a, err := accounts.GetAccount(owner)
		if err == nil {
			fromAcc = a
			break
		}
	}
	if fromAcc.Address == "" {
		appUI.Error("You don't have any wallet which is this multisig signer. Please jarvis wallet add to add the wallet.")
		return
	}
	fromAddr := fromAcc.Address

	amountStr, currency, err := util.ValueToAmountAndCurrency(value)
	if err != nil {
		appUI.Error("Wrong format of the --value/-v param")
		return
	}

	var tokenAddrLocal string
	if currency == util.ETH_ADDR || strings.EqualFold(currency, config.Network().GetNativeTokenSymbol()) {
		tokenAddrLocal = util.ETH_ADDR
	} else {
		addr, _, err := resolver.GetMatchingAddress(currency + " token")
		if err != nil {
			if util.IsAddress(currency) {
				tokenAddrLocal = currency
			} else {
				appUI.Error("Couldn't find the token by name or address")
				return
			}
		} else {
			tokenAddrLocal = addr
		}
	}

	toAddr, _, err := resolver.GetMatchingAddress(to)
	if err != nil {
		appUI.Error("Couldn't get destination address with keyword: %s", to)
		return
	}

	gasPrice := config.GasPrice
	if gasPrice == 0 {
		gasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			appUI.Error("Couldn't get recommended gas price: %s", err)
			return
		}
	}

	// Resolve amountWei — must happen regardless of whether the user supplied a gas limit.
	var amountWei *big.Int
	if tokenAddrLocal == util.ETH_ADDR {
		if amountStr == "ALL" {
			ethBalance, err := reader.GetBalance(msigContractAddr)
			if err != nil {
				appUI.Error("Couldn't get balance of the multisig: %s", err)
				return
			}
			amountWei = ethBalance
		} else {
			amountWei, err = jarviscommon.FloatStringToBig(amountStr, config.Network().GetNativeTokenDecimal())
			if err != nil {
				appUI.Error("Couldn't calculate the amount: %s", err)
				return
			}
		}
	} else {
		if amountStr == "ALL" {
			amountWei, err = reader.ERC20Balance(tokenAddrLocal, msigContractAddr)
			if err != nil {
				appUI.Error("Couldn't read balance of the multisig: %s", err)
				return
			}
		} else {
			decimals, err := reader.ERC20Decimal(tokenAddrLocal)
			if err != nil {
				appUI.Error("Couldn't get token decimal: %s", err)
				return
			}
			amountWei, err = jarviscommon.FloatStringToBig(amountStr, decimals)
			if err != nil {
				appUI.Error("Couldn't calculate amount in wei: %s", err)
				return
			}
		}
	}

	// Pack txdata — also must happen regardless of gas limit.
	var txdata []byte
	msigABI := util.GetGnosisMsigABI()
	if tokenAddrLocal == util.ETH_ADDR {
		txdata, err = msigABI.Pack(
			"submitTransaction",
			jarviscommon.HexToAddress(toAddr),
			amountWei,
			cmdutil.StringParamToBytes(data),
		)
		if err != nil {
			appUI.Error("Couldn't pack tx data: %s", err)
			return
		}
	} else {
		innerData, err := jarviscommon.PackERC20Data("transfer", jarviscommon.HexToAddress(toAddr), amountWei)
		if err != nil {
			appUI.Error("Couldn't pack transfer data: %s", err)
			return
		}
		txdata, err = msigABI.Pack(
			"submitTransaction",
			jarviscommon.HexToAddress(tokenAddrLocal),
			big.NewInt(0),
			innerData,
		)
		if err != nil {
			appUI.Error("Couldn't pack tx data: %s", err)
			return
		}
	}

	// Gas estimation — only when the user has not provided a value.
	gasLimit := config.GasLimit
	if gasLimit == 0 {
		if tokenAddrLocal == util.ETH_ADDR {
			gasLimit, err = reader.EstimateExactGas(fromAddr, msigContractAddr, 0, big.NewInt(0), txdata)
		} else {
			gasLimit, err = reader.EstimateGas(fromAddr, msigContractAddr, gasPrice+config.ExtraGasPrice, 0, txdata)
		}
		if err != nil {
			appUI.Error("Couldn't estimate gas: %s", err)
			return
		}
	}

	nonce := config.Nonce
	if nonce == 0 {
		nonce, err = reader.GetMinedNonce(fromAddr)
		if err != nil {
			appUI.Error("Couldn't get nonce of %s: %s", fromAddr, err)
			return
		}
	}

	txType, err := cmdutil.ValidTxType(reader, config.Network())
	if err != nil {
		appUI.Error("Couldn't determine proper tx type: %s", err)
		return
	}

	sp := sendTxParams{
		txType:   txType,
		nonce:    nonce,
		gasLimit: gasLimit + config.ExtraGasLimit,
		gasPrice: gasPrice + config.ExtraGasPrice,
		tipGas:   config.TipGas + config.ExtraTipGas,
	}
	handleMsigSend(sp, fromAcc, msigContractAddr, txdata, reader, analyzer, bc)
}

var sendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send eth or erc20 token from your account/multisig to others",
	Long: `Send eth or erc20 token from your account or multisig to other accounts.
The token and accounts can be specified either by memorable name or
exact addresses start with 0x.`,
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonSendPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)

		reader := tc.Reader
		bc := tc.Broadcaster
		analyzer := tc.Analyzer
		resolver := tc.Resolver
		if reader == nil {
			appUI.Error("Couldn't establish connection to node.")
			return
		}

		// The send command intentionally sets ExtraGasLimit to 0; other
		// commands default it to 250000.
		extraGasLimit := config.ExtraGasLimit
		if extraGasLimit == 250000 {
			extraGasLimit = 0
		}

		acc, err := accounts.GetAccount(config.From)
		if err != nil {
			// --from didn't match any local wallet, so it must be a
			// multisig the user is acting on behalf of. Decide between
			// Gnosis Safe and Gnosis Classic by probing the on-chain
			// shape of the contract; both flows then take care of
			// finding a local wallet that's actually an owner.
			if sc, ok := detectSafeForSend(reader, resolver, config.From); ok {
				sendFromSafe(reader, analyzer, resolver, bc, sc)
				return
			}
			sendFromMsig(reader, analyzer, resolver, bc)
			return
		}

		fromAcc := acc
		fromAddr := fromAcc.Address

		amountStr, currency, err := util.ValueToAmountAndCurrency(value)
		if err != nil {
			appUI.Error("Wrong format of --value/-v param")
			return
		}

		var tokenAddrLocal string
		if currency == util.ETH_ADDR || strings.EqualFold(currency, config.Network().GetNativeTokenSymbol()) {
			tokenAddrLocal = util.ETH_ADDR
		} else {
			addr, _, err := resolver.GetMatchingAddress(currency + " token")
			if err != nil {
				if util.IsAddress(currency) {
					tokenAddrLocal = currency
				} else {
					appUI.Error("Couldn't find the token by name or address")
					return
				}
			} else {
				tokenAddrLocal = addr
			}
		}

		toAddr, _, err := resolver.GetMatchingAddress(to)
		if err != nil {
			appUI.Error("Couldn't find destination address by keyword nor address: %s", to)
			return
		}

		gasPrice := config.GasPrice
		if gasPrice == 0 {
			gasPrice, err = reader.RecommendedGasPrice()
			if err != nil {
				appUI.Error("Couldn't estimate recommended gas price: %s", err)
				return
			}
		}

		txType, err := cmdutil.ValidTxType(reader, config.Network())
		if err != nil {
			appUI.Error("Couldn't determine proper tx type: %s", err)
			return
		}

		if txType == types.LegacyTxType && config.TipGas > 0 {
			appUI.Warn("We are doing legacy tx hence we ignore tip gas parameter.")
			return
		}

		tipGas := config.TipGas
		if txType == types.DynamicFeeTxType && tipGas == 0 {
			tipGas, err = reader.GetSuggestedGasTipCap()
			if err != nil {
				appUI.Error("Couldn't estimate recommended gas price: %s", err)
				return
			}
		}

		var amountWei *big.Int
		gasLimit := config.GasLimit
		if gasLimit == 0 {
			if tokenAddrLocal == util.ETH_ADDR {
				if amountStr == "ALL" {
					gasLimit, err = reader.EstimateExactGas(fromAddr, toAddr, 0, big.NewInt(1), cmdutil.StringParamToBytes(data))
					if err != nil {
						appUI.Error("Getting estimated gas for the tx failed: %s", err)
						return
					}
					extraGasLimit = 0 // exact gas for ALL; no extra needed

					ethBalance, err := reader.GetBalance(fromAddr)
					if err != nil {
						appUI.Error("Couldn't get %s balance: %s", config.Network().GetNativeTokenSymbol(), err)
						return
					}
					gasCost := big.NewInt(0).Mul(
						big.NewInt(int64(gasLimit)),
						jarviscommon.FloatToBigInt(gasPrice+config.ExtraGasPrice, 9),
					)
					if ethBalance.Cmp(gasCost) == -1 {
						appUI.Error("Wallet doesn't have enough token to cover gas. Aborted.")
						return
					}
					amountWei = big.NewInt(0).Sub(ethBalance, gasCost)
				} else {
					amountWei, err = jarviscommon.FloatStringToBig(amountStr, config.Network().GetNativeTokenDecimal())
					if err != nil {
						appUI.Error("Couldn't calculate send amount: %s", err)
						return
					}
					gasLimit, err = reader.EstimateExactGas(fromAddr, toAddr, 0, amountWei, cmdutil.StringParamToBytes(data))
					if err != nil {
						appUI.Error("Getting estimated gas for the tx failed: %s", err)
						return
					}
				}
			} else {
				var innerData []byte
				if amountStr == "ALL" {
					amountWei, err = reader.ERC20Balance(tokenAddrLocal, fromAddr)
					if err != nil {
						appUI.Error("Couldn't get token balance: %s", err)
						return
					}
					innerData, err = jarviscommon.PackERC20Data("transfer", jarviscommon.HexToAddress(toAddr), amountWei)
					if err != nil {
						appUI.Error("Couldn't pack data: %s", err)
						return
					}
				} else {
					decimals, err := reader.ERC20Decimal(tokenAddrLocal)
					if err != nil {
						appUI.Error("Couldn't get token decimal: %s", err)
						return
					}
					amountWei, err = jarviscommon.FloatStringToBig(amountStr, decimals)
					if err != nil {
						appUI.Error("Couldn't calculate token amount in wei: %s", err)
						return
					}
					innerData, err = jarviscommon.PackERC20Data("transfer", jarviscommon.HexToAddress(toAddr), amountWei)
					if err != nil {
						appUI.Error("Couldn't pack data: %s", err)
						return
					}
				}
				gasLimit, err = reader.EstimateGas(fromAddr, tokenAddrLocal, gasPrice+config.ExtraGasPrice, 0, innerData)
				if err != nil {
					appUI.Error("Couldn't estimate gas limit: %s", err)
					return
				}
			}
		}

		nonce := config.Nonce
		if nonce == 0 {
			nonce, err = reader.GetMinedNonce(fromAddr)
			if err != nil {
				appUI.Error("Couldn't get nonce: %s", err)
				return
			}
		}

		sp := sendTxParams{
			txType:   txType,
			nonce:    nonce,
			gasLimit: gasLimit + extraGasLimit,
			gasPrice: gasPrice + config.ExtraGasPrice,
			tipGas:   tipGas + config.ExtraTipGas,
		}
		handleSend(sp, fromAcc, toAddr, amountWei, tokenAddrLocal, data, reader, analyzer, bc)
	},
}

// detectSafeForSend resolves keyword (an address, jarvis name, EIP-3770
// short reference, or Safe-app URL) to a SafeContract, and returns true
// only when the unified multisig detector reports the address as a Safe.
// It is intentionally silent on misses so the caller can fall back to
// Gnosis Classic detection without polluting the output.
//
// We delegate to cmdutil.DetectMultisigType so the result is shared with
// the on-disk type cache populated by `jarvis msig` commands — running
// `jarvis send --from <safe>` after a previous `jarvis msig info <safe>`
// avoids paying the on-chain probe again.
func detectSafeForSend(
	reader utilreader.Reader,
	resolver cmdutil.ABIResolver,
	keyword string,
) (*safe.SafeContract, bool) {
	candidate := keyword
	if ref, ok := safe.ParseSafeAppURL(keyword); ok && ref.SafeAddress != (ethcommon.Address{}) {
		candidate = ref.SafeAddress.Hex()
	}

	addr, _, err := resolver.GetMatchingAddress(candidate)
	if err != nil {
		addresses := util.ScanForAddresses(candidate)
		if len(addresses) == 0 {
			return nil, false
		}
		addr = addresses[0]
	}

	typ, err := cmdutil.DetectMultisigType(config.Network(), addr)
	if err != nil || typ != cmdutil.MultisigSafe {
		return nil, false
	}
	sc, err := safe.NewSafeContract(addr, config.Network())
	if err != nil {
		return nil, false
	}
	return sc, true
}

// sendFromSafe is the Gnosis-Safe analogue of sendFromMsig: it builds a
// SafeTx (native transfer or ERC20 transfer) targeting --to with the
// given amount, signs the EIP-712 safeTxHash with the single local
// wallet that's also a Safe owner, and submits the proposal to the Safe
// Transaction Service. The print-out matches `jarvis safe init` so the
// follow-up commands (approve / execute) are immediately discoverable.
func sendFromSafe(
	reader utilreader.Reader,
	analyzer util.TxAnalyzer,
	resolver cmdutil.ABIResolver,
	bc cmdutil.TxBroadcaster,
	safeContract *safe.SafeContract,
) {
	appUI.Section("Safe info")
	appUI.Info("Safe address : %s", safeContract.Address)
	if v, err := safeContract.Version(); err == nil {
		appUI.Info("Safe version : %s", v)
	}
	if t, err := safeContract.Threshold(); err == nil {
		appUI.Info("Threshold    : %d", t)
	}

	owners, err := safeContract.Owners()
	if err != nil {
		appUI.Error("getting safe owners failed: %s", err)
		return
	}

	var fromAcc types2.AccDesc
	var matchingOwners int
	for _, owner := range owners {
		acc, err := accounts.GetAccount(owner)
		if err == nil {
			fromAcc = acc
			matchingOwners++
		}
	}
	if matchingOwners == 0 {
		appUI.Error("You don't have any wallet that is an owner of this Safe. Please run `jarvis wallet add` first.")
		return
	}
	if matchingOwners > 1 {
		appUI.Error("You have multiple wallets that are owners of this Safe; please pass --from explicitly.")
		return
	}
	fromAddr := fromAcc.Address

	amountStr, currency, err := util.ValueToAmountAndCurrency(value)
	if err != nil {
		appUI.Error("Wrong format of the --value/-v param")
		return
	}

	var tokenAddrLocal string
	if currency == util.ETH_ADDR || strings.EqualFold(currency, config.Network().GetNativeTokenSymbol()) {
		tokenAddrLocal = util.ETH_ADDR
	} else {
		addr, _, err := resolver.GetMatchingAddress(currency + " token")
		if err != nil {
			if util.IsAddress(currency) {
				tokenAddrLocal = currency
			} else {
				appUI.Error("Couldn't find the token by name or address")
				return
			}
		} else {
			tokenAddrLocal = addr
		}
	}

	toAddr, _, err := resolver.GetMatchingAddress(to)
	if err != nil {
		appUI.Error("Couldn't get destination address with keyword: %s", to)
		return
	}

	// Compute the wei amount the Safe should move. ALL refers to the
	// Safe's balance, NOT the EOA's, mirroring sendFromMsig semantics.
	var amountWei *big.Int
	if tokenAddrLocal == util.ETH_ADDR {
		if amountStr == "ALL" {
			ethBalance, err := reader.GetBalance(safeContract.Address)
			if err != nil {
				appUI.Error("Couldn't get balance of the safe: %s", err)
				return
			}
			amountWei = ethBalance
		} else {
			amountWei, err = jarviscommon.FloatStringToBig(amountStr, config.Network().GetNativeTokenDecimal())
			if err != nil {
				appUI.Error("Couldn't calculate the amount: %s", err)
				return
			}
		}
	} else {
		if amountStr == "ALL" {
			amountWei, err = reader.ERC20Balance(tokenAddrLocal, safeContract.Address)
			if err != nil {
				appUI.Error("Couldn't read token balance of the safe: %s", err)
				return
			}
		} else {
			decimals, err := reader.ERC20Decimal(tokenAddrLocal)
			if err != nil {
				appUI.Error("Couldn't get token decimal: %s", err)
				return
			}
			amountWei, err = jarviscommon.FloatStringToBig(amountStr, decimals)
			if err != nil {
				appUI.Error("Couldn't calculate amount in wei: %s", err)
				return
			}
		}
	}

	// Inner SafeTx call: native send carries (to, value, --data); ERC20
	// transfer wraps the value into ERC20.transfer calldata against the
	// token contract while value=0.
	var (
		safeTo    ethcommon.Address
		safeValue *big.Int
		safeData  []byte
	)
	if tokenAddrLocal == util.ETH_ADDR {
		safeTo = ethcommon.HexToAddress(toAddr)
		safeValue = amountWei
		safeData = cmdutil.StringParamToBytes(data)
	} else {
		erc20Data, err := jarviscommon.PackERC20Data("transfer", jarviscommon.HexToAddress(toAddr), amountWei)
		if err != nil {
			appUI.Error("Couldn't pack ERC20 transfer data: %s", err)
			return
		}
		safeTo = ethcommon.HexToAddress(tokenAddrLocal)
		safeValue = big.NewInt(0)
		safeData = erc20Data
	}

	collector, err := safe.NewTxServiceCollector(config.Network().GetChainID())
	if err != nil {
		appUI.Error("Couldn't init Safe Transaction Service client for chain %d: %s", config.Network().GetChainID(), err)
		return
	}

	safeNonce, err := nextSafeNonce(safeContract, collector)
	if err != nil {
		appUI.Error("Couldn't determine the next safe nonce: %s", err)
		return
	}
	appUI.Info("SafeTx nonce: %d", safeNonce)

	domainSep, err := safeContract.DomainSeparator()
	if err != nil {
		appUI.Error("Couldn't read on-chain domainSeparator: %s", err)
		return
	}

	stx := safe.NewSafeTx(safeTo, safeValue, safeData, safe.OpCall, safeNonce)
	hash := stx.SafeTxHash(domainSep)

	// Synthesise a TxContext just rich enough for showSafeTxToConfirm to
	// resolve the destination ABI and decode the inner calldata. The
	// fields we omit (FromAcc, Broadcaster, etc.) are not consulted by
	// the display path.
	tcView := cmdutil.TxContext{
		Reader:   reader,
		Analyzer: analyzer,
		Resolver: resolver,
	}
	showSafeTxToConfirm(stx, hash, &tcView)

	if !config.YesToAllPrompt && !appUI.Confirm("Sign and submit this Safe transaction?", true) {
		appUI.Warn("Aborted by user.")
		return
	}

	appUI.Info("Unlock %s and sign the EIP-712 safeTxHash now...", fromAddr)
	account, err := accounts.UnlockAccount(fromAcc)
	if err != nil {
		appUI.Error("Couldn't unlock wallet: %s", err)
		if errors.Is(err, cmdutil.ErrWalletUnlock) {
			os.Exit(126)
		}
		return
	}

	structHash := stx.StructHash()
	sig, err := account.SignSafeHash(domainSep, structHash)
	if err != nil {
		appUI.Error("Couldn't sign safeTxHash: %s", err)
		return
	}

	if err := collector.Propose(
		ethcommon.HexToAddress(safeContract.Address),
		stx, hash,
		ethcommon.HexToAddress(fromAddr),
		sig,
	); err != nil {
		appUI.Error("Submitting proposal to Safe Transaction Service failed: %s", err)
		return
	}

	appUI.Success("Proposal submitted.")
	appUI.Info("safeTxHash: 0x%s", ethcommon.Bytes2Hex(hash[:]))
	appUI.Info("Other owners can approve with:")
	appUI.Info("  jarvis safe approve %s 0x%s", safeContract.Address, ethcommon.Bytes2Hex(hash[:]))
	appUI.Info("Once threshold is met, anyone can execute with:")
	appUI.Info("  jarvis safe execute %s 0x%s", safeContract.Address, ethcommon.Bytes2Hex(hash[:]))
}

func init() {
	AddCommonFlagsToTransactionalCmds(sendCmd)
	sendCmd.Flags().StringVarP(&to, "to", "t", "", "Account to send eth to. It can be ethereum address or a hint string to look it up in the address database. See jarvis addr for all of the known addresses")
	sendCmd.Flags().StringVarP(&value, "amount", "v", "0", "Amount of eth to send. It is in eth/token value, not wei/twei. If a float number is passed, it will be interpreted as ETH, otherwise, it must be in the form of `float|ALL address` or `float|ALL name`. In the later case, `name` will be used to look for the token address. Eg. 0.01, 0.01 knc, 0.01 0xdd974d5c2e2928dea5f71b9825b8b646686bd200, ALL KNC are valid values.")
	sendCmd.Flags().StringVarP(&data, "data", "D", "", "Data to send along with the transaction. It is in hex format.")
	sendCmd.MarkFlagRequired("to")
	sendCmd.MarkFlagRequired("amount")

	rootCmd.AddCommand(sendCmd)
}
