package cmd

import (
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	types2 "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/msig"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

var (
	to        string
	amountStr string
	amountWei *big.Int
	value     string
	tokenAddr string
	currency  string
	data      string
)

func handleMsigSend(
	txType uint8,
	from types2.AccDesc,
	to string,
	txdata []byte,
) {
	var (
		t *types.Transaction
		a *abi.ABI
	)

	reader, err := util.EthReader(config.Network())
	if err != nil {
		appUI.Error("init reader failed: %s", err)
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	bc, err := util.EthBroadcaster(config.Network())
	if err != nil {
		appUI.Error("init broadcaster failed: %s", err)
		return
	}

	t = jarviscommon.BuildExactTx(
		txType,
		config.Nonce,
		to,
		big.NewInt(0),
		config.GasLimit+config.ExtraGasLimit,
		config.GasPrice+config.ExtraGasPrice,
		config.TipGas+config.ExtraTipGas,
		txdata,
		config.Network().GetChainID(),
	)

	err = cmdutil.PromptTxConfirmation(
		appUI,
		analyzer,
		util.GetJarvisAddress(config.From, config.Network()),
		t,
		nil,
		config.Network(),
	)
	if err != nil {
		appUI.Error("Aborted!")
		return
	}

	appUI.Info("Unlock your wallet and send now...")
	account, err := accounts.UnlockAccount(from)
	if err != nil {
		appUI.Error("Failed: %s", err)
		os.Exit(126)
	}

	signedAddr, signedTx, err := account.SignTx(t, big.NewInt(int64(config.Network().GetChainID())))
	if err != nil {
		appUI.Error("failed to sign tx: %s", err)
		return
	}
	if signedAddr.Cmp(jarviscommon.HexToAddress(from.Address)) != 0 {
		appUI.Error(
			"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
			from.Address,
			signedAddr.Hex(),
		)
		return
	}

	broadcasted, err := cmdutil.HandlePostSign(appUI, signedTx, reader, analyzer, a, bc)
	if err != nil && !broadcasted {
		appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

// currency here is supposed to be either ETH or address of an ERC20 token
func handleSend(
	txType uint8,
	from types2.AccDesc,
	to string,
	amountWei *big.Int,
	tokenAddr string,
) {
	var (
		t *types.Transaction
		a *abi.ABI
	)

	reader, err := util.EthReader(config.Network())
	if err != nil {
		appUI.Error("init reader failed: %s", err)
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	bc, err := util.EthBroadcaster(config.Network())
	if err != nil {
		appUI.Error("init broadcaster failed: %s", err)
		return
	}

	if tokenAddr == util.ETH_ADDR {
		t = jarviscommon.BuildExactTx(
			txType,
			config.Nonce,
			to,
			amountWei,
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			config.TipGas+config.ExtraTipGas,
			cmdutil.StringParamToBytes(data),
			config.Network().GetChainID(),
		)
	} else {
		a = jarviscommon.GetERC20ABI()
		data, err := a.Pack(
			"transfer",
			jarviscommon.HexToAddress(to),
			amountWei,
		)
		if err != nil {
			appUI.Error("Couldn't pack data: %s", err)
			return
		}
		t = jarviscommon.BuildExactTx(
			txType,
			config.Nonce,
			tokenAddr,
			big.NewInt(0),
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			config.TipGas+config.ExtraTipGas,
			data,
			config.Network().GetChainID(),
		)
	}

	err = cmdutil.PromptTxConfirmation(
		appUI,
		analyzer,
		util.GetJarvisAddress(config.From, config.Network()),
		t,
		map[string]*abi.ABI{
			strings.ToLower(tokenAddr): a,
		},
		config.Network(),
	)
	if err != nil {
		appUI.Error("Aborted!")
		return
	}

	appUI.Info("Unlock your wallet and send now...")
	account, err := accounts.UnlockAccount(from)
	if err != nil {
		appUI.Error("Failed: %s", err)
		os.Exit(126)
	}

	signedAddr, signedTx, err := account.SignTx(t, big.NewInt(int64(config.Network().GetChainID())))
	if err != nil {
		appUI.Error("Failed to sign tx: %s", err)
		return
	}
	if signedAddr.Cmp(jarviscommon.HexToAddress(from.Address)) != 0 {
		appUI.Error(
			"Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s",
			from.Address,
			signedAddr.Hex(),
		)
		return
	}

	broadcasted, err := cmdutil.HandlePostSign(appUI, signedTx, reader, analyzer, a, bc)
	if err != nil && !broadcasted {
		appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
	}
}

func sendFromMsig() {
	msigAddress, err := getMsigContractFromParams([]string{config.From})
	if err != nil {
		appUI.Error("Couldn't find a wallet or multisig with keyword %s", config.From)
		return
	}

	config.To, _, err = util.GetAddressFromString(msigAddress)
	if err != nil {
		appUI.Error("Couldn't find a wallet or multisig with keyword \"%s\"", config.From)
		return
	}

	multisigContract, err := msig.NewMultisigContract(
		config.To,
		config.Network(),
	)
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
	count := 0
	for _, owner := range owners {
		a, err := accounts.GetAccount(owner)
		if err == nil {
			fromAcc = a
			count++
			break
		}
	}
	if count == 0 {
		appUI.Error("You don't have any wallet which is this multisig signer. Please jarvis wallet add to add the wallet.")
		return
	}

	config.From = fromAcc.Address

	amountStr, currency, err := util.ValueToAmountAndCurrency(value)
	if err != nil {
		appUI.Error("Wrong format of the --value/-v param")
		return
	}

	if currency == util.ETH_ADDR || strings.EqualFold(currency, config.Network().GetNativeTokenSymbol()) {
		tokenAddr = util.ETH_ADDR
	} else {
		addr, _, err := util.GetMatchingAddress(currency + " token")
		if err != nil {
			if util.IsAddress(currency) {
				tokenAddr = currency
			} else {
				appUI.Error("Couldn't find the token by name or address")
				return
			}
		} else {
			tokenAddr = addr
		}
	}

	toAddr, _, err := util.GetMatchingAddress(to)
	if err != nil {
		appUI.Error("Couldn't get destination address with keyword: %s", to)
		return
	}
	to = toAddr

	reader, err := util.EthReader(config.Network())
	if err != nil {
		appUI.Error("Couldn't init connection to node: %s", err)
		return
	}

	if config.GasPrice == 0 {
		config.GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			appUI.Error("Couldn't get recommended gas price: %s", err)
			return
		}
	}

	var txdata []byte

	if config.GasLimit == 0 {
		if tokenAddr == util.ETH_ADDR {
			if amountStr == "ALL" {
				ethBalance, err := reader.GetBalance(config.To)
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

			msigABI := util.GetGnosisMsigABI()
			txdata, err = msigABI.Pack(
				"submitTransaction",
				jarviscommon.HexToAddress(to),
				amountWei,
				cmdutil.StringParamToBytes(data),
			)
			if err != nil {
				appUI.Error("Couldn't pack tx data 1: %s", err)
				return
			}

			config.GasLimit, err = reader.EstimateExactGas(config.From, config.To, 0, big.NewInt(0), txdata)
			if err != nil {
				appUI.Error("Getting estimated gas for the tx failed: %s", err)
				return
			}
		} else {
			var innerData []byte
			if amountStr == "ALL" {
				amountWei, err = reader.ERC20Balance(tokenAddr, config.To)
				if err != nil {
					appUI.Error("Couldn't read balance of the multisig: %s", err)
				}
				innerData, err = jarviscommon.PackERC20Data(
					"transfer",
					jarviscommon.HexToAddress(to),
					amountWei,
				)
				if err != nil {
					appUI.Error("Couldn't pack transfer data: %s", err)
					return
				}
			} else {
				decimals, err := reader.ERC20Decimal(tokenAddr)
				if err != nil {
					appUI.Error("Couldn't get token decimal: %s", err)
					return
				}
				amountWei, err = jarviscommon.FloatStringToBig(amountStr, decimals)
				if err != nil {
					appUI.Error("Couldn't calculate amount in wei: %s", err)
					return
				}
				innerData, err = jarviscommon.PackERC20Data(
					"transfer",
					jarviscommon.HexToAddress(to),
					amountWei,
				)
				if err != nil {
					appUI.Error("Couldn't pack transfer data: %s", err)
					return
				}
			}

			msigABI := util.GetGnosisMsigABI()
			txdata, err = msigABI.Pack(
				"submitTransaction",
				jarviscommon.HexToAddress(tokenAddr),
				big.NewInt(0),
				innerData,
			)
			if err != nil {
				appUI.Error("Couldn't pack tx data 2: %s", err)
				return
			}
			config.GasLimit, err = reader.EstimateGas(config.From, config.To, config.GasPrice+config.ExtraGasPrice, 0, txdata)
			if err != nil {
				appUI.Error("Couldn't estimate gas: %s", err)
				return
			}
		}
	}

	if config.Nonce == 0 {
		config.Nonce, err = reader.GetMinedNonce(config.From)
		if err != nil {
			appUI.Error("Couldn't get nonce of %s: %s", config.From, err)
			return
		}
	}

	txType, err := cmdutil.ValidTxType(reader, config.Network())
	if err != nil {
		appUI.Error("Couldn't determine proper tx type: %s", err)
		return
	}

	handleMsigSend(txType, fromAcc, config.To, txdata)
}

func init() {
	sendCmd := &cobra.Command{
		Use:   "send",
		Short: "Send eth or erc20 token from your account/multisig to others",
		Long: `Send eth or erc20 token from your account or multisig to other accounts.
The token and accounts can be specified either by memorable name or
exact addresses start with 0x.`,
		TraverseChildren: true,
		Run: func(cmd *cobra.Command, args []string) {
			err := config.SetNetwork(config.NetworkString)
			if err != nil {
				appUI.Error("network param is wrong: %s", err)
				return
			}
			appUI.Info("Network: %s", config.Network().GetName())

			if config.ExtraGasLimit == 250000 {
				config.ExtraGasLimit = 0
			}

			acc, err := accounts.GetAccount(config.From)
			if err != nil {
				sendFromMsig()
				return
			}

			fromAcc := acc
			config.From = acc.Address

			amountStr, currency, err = util.ValueToAmountAndCurrency(value)
			if err != nil {
				appUI.Error("Wrong format of --value/-v param")
				return
			}

			if currency == util.ETH_ADDR || strings.EqualFold(currency, config.Network().GetNativeTokenSymbol()) {
				tokenAddr = util.ETH_ADDR
			} else {
				addr, _, err := util.GetMatchingAddress(currency + " token")
				if err != nil {
					if util.IsAddress(currency) {
						tokenAddr = currency
					} else {
						appUI.Error("Couldn't find the token by name or address")
						return
					}
				} else {
					tokenAddr = addr
				}
			}

			toAddr, _, err := util.GetMatchingAddress(to)
			if err != nil {
				appUI.Error("Couldn't find destination address by keyword nor address: %s", to)
				return
			}
			to = toAddr

			reader, err := util.EthReader(config.Network())
			if err != nil {
				appUI.Error("Couldn't establish connection to node: %s", err)
				return
			}
			if config.GasPrice == 0 {
				config.GasPrice, err = reader.RecommendedGasPrice()
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

			if txType == types.DynamicFeeTxType {
				if config.TipGas == 0 {
					config.TipGas, err = reader.GetSuggestedGasTipCap()
					if err != nil {
						appUI.Error("Couldn't estimate recommended gas price: %s", err)
						return
					}
				}
			}

			if config.GasLimit == 0 {
				if tokenAddr == util.ETH_ADDR {
					if amountStr == "ALL" {
						config.GasLimit, err = reader.EstimateExactGas(config.From, to, 0, big.NewInt(1), cmdutil.StringParamToBytes(data))
						if err != nil {
							appUI.Error("Getting estimated gas for the tx failed: %s", err)
							return
						}
						config.ExtraGasLimit = 0

						ethBalance, err := reader.GetBalance(config.From)
						if err != nil {
							appUI.Error("Couldn't get %s balance: %s", config.Network().GetNativeTokenSymbol(), err)
							return
						}
						gasCost := big.NewInt(0).Mul(
							big.NewInt(int64(config.GasLimit)),
							jarviscommon.FloatToBigInt(config.GasPrice+config.ExtraGasPrice, 9),
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
						config.GasLimit, err = reader.EstimateExactGas(config.From, to, 0, amountWei, cmdutil.StringParamToBytes(data))
						if err != nil {
							appUI.Error("Getting estimated gas for the tx failed: %s", err)
							return
						}
					}
				} else {
					var innerData []byte
					if amountStr == "ALL" {
						amountWei, err = reader.ERC20Balance(tokenAddr, config.From)
						if err != nil {
							appUI.Error("Couldn't get token balance: %s", err)
							return
						}
						innerData, err = jarviscommon.PackERC20Data(
							"transfer",
							jarviscommon.HexToAddress(to),
							amountWei,
						)
						if err != nil {
							appUI.Error("Couldn't pack data: %s", err)
							return
						}
					} else {
						decimals, err := reader.ERC20Decimal(tokenAddr)
						if err != nil {
							appUI.Error("Couldn't get token decimal: %s", err)
							return
						}
						amountWei, err = jarviscommon.FloatStringToBig(amountStr, decimals)
						if err != nil {
							appUI.Error("Couldn't calculate token amount in wei: %s", err)
							return
						}
						innerData, err = jarviscommon.PackERC20Data(
							"transfer",
							jarviscommon.HexToAddress(to),
							amountWei,
						)
						if err != nil {
							appUI.Error("Couldn't pack data: %s", err)
							return
						}
					}
					config.GasLimit, err = reader.EstimateGas(config.From, tokenAddr, config.GasPrice+config.ExtraGasPrice, 0, innerData)
					if err != nil {
						appUI.Error("Couldn't estimate gas limit: %s", err)
						return
					}
				}
			}

			if config.Nonce == 0 {
				config.Nonce, err = reader.GetMinedNonce(config.From)
				if err != nil {
					appUI.Error("Couldn't get nonce: %s", err)
					return
				}
			}

			handleSend(
				txType,
				fromAcc,
				to,
				amountWei,
				tokenAddr,
			)
		},
	}

	AddCommonFlagsToTransactionalCmds(sendCmd)
	sendCmd.Flags().StringVarP(&to, "to", "t", "", "Account to send eth to. It can be ethereum address or a hint string to look it up in the address database. See jarvis addr for all of the known addresses")
	sendCmd.Flags().StringVarP(&value, "amount", "v", "0", "Amount of eth to send. It is in eth/token value, not wei/twei. If a float number is passed, it will be interpreted as ETH, otherwise, it must be in the form of `float|ALL address` or `float|ALL name`. In the later case, `name` will be used to look for the token address. Eg. 0.01, 0.01 knc, 0.01 0xdd974d5c2e2928dea5f71b9825b8b646686bd200, ALL KNC are valid values.")
	sendCmd.Flags().StringVarP(&data, "data", "D", "", "Data to send along with the transaction. It is in hex format.")
	sendCmd.MarkFlagRequired("to")
	sendCmd.MarkFlagRequired("amount")

	rootCmd.AddCommand(sendCmd)
}
