package cmd

import (
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	types2 "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	. "github.com/tranvictor/jarvis/common"
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
	err       error
)

func handleMsigSend(
	txType uint8,
	cmd *cobra.Command, args []string,
	basePrice, extraPrice float64,
	baseGas, extraGas uint64,
	nonce uint64,
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
		fmt.Printf("init reader failed: %s\n", err)
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	t = BuildExactTx(
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
		analyzer,
		util.GetJarvisAddress(config.From, config.Network()),
		t,
		nil,
		config.Network(),
	)
	if err != nil {
		fmt.Printf("Aborted!\n")
		return
	}

	fmt.Printf("== Unlock your wallet and send now...\n")
	account, err := accounts.UnlockAccount(from)
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		os.Exit(126)
	}

	signedTx, err := account.SignTx(t, big.NewInt(int64(config.Network().GetChainID())))
	if err != nil {
		fmt.Printf("Failed to sign tx: %s\n", err)
		return
	}

	broadcasted, err := cmdutil.HandlePostSign(signedTx, reader, analyzer, a)
	if err != nil && !broadcasted {
		fmt.Printf("Failed to proceed after signing the tx: %s. Aborted.\n", err)
	}
}

// currency here is supposed to be either ETH or address of an ERC20 token
func handleSend(
	txType uint8,
	cmd *cobra.Command, args []string,
	basePrice, extraPrice float64,
	baseGas, extraGas uint64,
	nonce uint64,
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
		fmt.Printf("init reader failed: %s\n", err)
		return
	}

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	if tokenAddr == util.ETH_ADDR {
		t = BuildExactTx(
			txType,
			config.Nonce,
			to,
			amountWei,
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			config.TipGas+config.ExtraTipGas,
			[]byte{},
			config.Network().GetChainID(),
		)
	} else {
		a = GetERC20ABI()
		data, err := a.Pack(
			"transfer",
			HexToAddress(to),
			amountWei,
		)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		t = BuildExactTx(
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
		analyzer,
		util.GetJarvisAddress(config.From, config.Network()),
		t,
		map[string]*abi.ABI{
			strings.ToLower(tokenAddr): a,
		},
		config.Network(),
	)
	if err != nil {
		fmt.Printf("Aborted!\n")
		return
	}

	fmt.Printf("== Unlock your wallet and send now...\n")
	account, err := accounts.UnlockAccount(from)
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		os.Exit(126)
	}

	signedTx, err := account.SignTx(t, big.NewInt(int64(config.Network().GetChainID())))
	if err != nil {
		fmt.Printf("Failed to sign tx: %s\n", err)
		return
	}

	broadcasted, err := cmdutil.HandlePostSign(signedTx, reader, analyzer, a)
	if err != nil && !broadcasted {
		fmt.Printf("Failed to proceed after signing the tx: %s. Aborted.\n", err)
	}
}

func sendFromMsig(cmd *cobra.Command, args []string) {
	msigAddress, err := getMsigContractFromParams([]string{config.From})
	if err != nil {
		fmt.Printf("Couldn't find a wallet or multisig with keyword %s\n", config.From)
		return
	}

	config.To, _, err = util.GetAddressFromString(msigAddress)

	multisigContract, err := msig.NewMultisigContract(
		config.To,
		config.Network(),
	)
	if err != nil {
		fmt.Printf("Couldn't read the multisig: %s\n", err)
		return
	}

	owners, err := multisigContract.Owners()
	if err != nil {
		fmt.Printf("getting msig owners failed: %s\n", err)
		return
	}

	var acc types2.AccDesc
	count := 0
	for _, owner := range owners {
		a, err := accounts.GetAccount(owner)
		if err == nil {
			acc = a
			count++
			break
		}
	}
	if count == 0 {
		fmt.Printf("You don't have any wallet which is this multisig signer. Please jarvis wallet add to add the wallet.")
		return
	}

	config.FromAcc = acc
	config.From = acc.Address

	amountStr, currency, err := util.ValueToAmountAndCurrency(value)
	if err != nil {
		fmt.Printf("Wrong format of the --value/-v param\n")
		return
	}

	// if value is not an address, we need to look it up
	// from the token database to get its address

	if currency == util.ETH_ADDR || strings.ToLower(currency) == strings.ToLower(config.Network().GetNativeTokenSymbol()) {
		tokenAddr = util.ETH_ADDR
	} else {
		addr, _, err := util.GetMatchingAddress(fmt.Sprintf("%s token", currency))
		if err != nil {
			if util.IsAddress(currency) {
				tokenAddr = currency
			} else {
				fmt.Printf("Couldn't find the token by name or address\n")
				return
			}
		} else {
			tokenAddr = addr
		}
	}

	// process to to get address
	toAddr, _, err := util.GetMatchingAddress(to)
	if err != nil {
		fmt.Printf("Couldn't get destination address with keyword: %s\n", to)
		return
	} else {
		to = toAddr
	}

	reader, err := util.EthReader(config.Network())
	if err != nil {
		fmt.Printf("Couldn't init connection to node: %s\n", err)
		return
	}

	// var GasPrice float64
	if config.GasPrice == 0 {
		config.GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			fmt.Printf("Couldn't get recommended gas price: %s\n", err)
			return
		}
	}

	var txdata []byte

	// var GasLimit uint64
	if config.GasLimit == 0 {
		if tokenAddr == util.ETH_ADDR {
			if amountStr == "ALL" {
				ethBalance, err := reader.GetBalance(config.To)
				if err != nil {
					fmt.Printf("Couldn't get balance of the multisig: %s\n", err)
					return
				}
				amountWei = ethBalance
			} else {
				amountWei, err = FloatStringToBig(amountStr, config.Network().GetNativeTokenDecimal())
				if err != nil {
					fmt.Printf("Couldn't calculate the amount: %s\n", err)
					return
				}
			}

			msigABI := util.GetGnosisMsigABI()
			txdata, err = msigABI.Pack(
				"submitTransaction",
				HexToAddress(to),
				amountWei,
				[]byte{},
			)

			if err != nil {
				fmt.Printf("Couldn't pack tx data 1: %s\n", err)
				return
			}

			config.GasLimit, err = reader.EstimateExactGas(config.From, config.To, 0, big.NewInt(0), txdata)
			if err != nil {
				fmt.Printf("Getting estimated gas for the tx failed: %s\n", err)
				return
			}

		} else {
			var data []byte
			if amountStr == "ALL" {
				amountWei, err = reader.ERC20Balance(tokenAddr, config.To)
				if err != nil {
					fmt.Printf("Couldn't read balance of the multisig: %s\n", err)
				}
				data, err = PackERC20Data(
					"transfer",
					HexToAddress(to),
					amountWei,
				)
				if err != nil {
					fmt.Printf("Couldn't pack transfer data: %s\n", err)
					return
				}
			} else {
				decimals, err := reader.ERC20Decimal(tokenAddr)
				if err != nil {
					fmt.Printf("Couldn't get token decimal: %s\n", err)
					return
				}
				amountWei, err = FloatStringToBig(amountStr, decimals)
				if err != nil {
					fmt.Printf("Couldn't calculate amount in wei: %s\n", err)
					return
				}

				data, err = PackERC20Data(
					"transfer",
					HexToAddress(to),
					amountWei,
				)
				if err != nil {
					fmt.Printf("Couldn't pack transfer data: %s\n", err)
					return
				}
			}

			msigABI := util.GetGnosisMsigABI()
			txdata, err = msigABI.Pack(
				"submitTransaction",
				HexToAddress(tokenAddr),
				big.NewInt(0),
				data,
			)

			if err != nil {
				fmt.Printf("Couldn't pack tx data 2: %s\n", err)
				return
			}
			config.GasLimit, err = reader.EstimateGas(config.From, config.To, config.GasPrice+config.ExtraGasPrice, 0, txdata)
			if err != nil {
				fmt.Printf("Couldn't estimate gas: %s\n", err)
				return
			}
		}
	}

	// var Nonce uint64
	if config.Nonce == 0 {
		config.Nonce, err = reader.GetMinedNonce(config.From)
		if err != nil {
			fmt.Printf("Couldn't get nonce of %s: %s\n", config.From, err)
			return
		}
	}

	handleMsigSend(
		config.TxType,
		cmd, args,
		config.GasPrice, config.ExtraGasPrice,
		config.GasLimit, config.ExtraGasLimit,
		config.Nonce,
		config.FromAcc,
		config.To,
		txdata,
	)
}

func init() {
	// sendCmd represents the send command
	sendCmd := &cobra.Command{
		Use:   "send",
		Short: "Send eth or erc20 token from your account/multisig to others",
		Long: `Send eth or erc20 token from your account or multisig to other accounts.
The token and accounts can be specified either by memorable name or
exact addresses start with 0x.`,
		TraverseChildren: true,
		Run: func(cmd *cobra.Command, args []string) {
			// if extra gas is set to default value, we force it to 0 to ensure sending ETH
			// will leave no dust
			if config.ExtraGasLimit == 250000 {
				config.ExtraGasLimit = 0
			}

			// process from to get address
			acc, err := accounts.GetAccount(config.From)
			if err != nil {
				// if config.From is not in wallet list, fall back to msig send
				sendFromMsig(cmd, args)
				return
			}

			config.FromAcc = acc
			config.From = acc.Address

			amountStr, currency, err = util.ValueToAmountAndCurrency(value)
			if err != nil {
				fmt.Printf("Wrong format of --value/-v param\n")
				return
			}

			// if value is not an address, we need to look it up
			// from the token database to get its address

			if currency == util.ETH_ADDR || strings.ToLower(currency) == strings.ToLower(config.Network().GetNativeTokenSymbol()) {
				tokenAddr = util.ETH_ADDR
			} else {
				addr, _, err := util.GetMatchingAddress(fmt.Sprintf("%s token", currency))
				if err != nil {
					if util.IsAddress(currency) {
						tokenAddr = currency
					} else {
						fmt.Printf("Couldn't find the token by name or address\n")
						return
					}
				} else {
					tokenAddr = addr
				}
			}

			// process to to get address
			toAddr, _, err := util.GetMatchingAddress(to)
			if err != nil {
				fmt.Printf("Couldn't find destination address by keyword nor address: %s\n", to)
				return
			} else {
				to = toAddr
			}

			reader, err := util.EthReader(config.Network())
			if err != nil {
				fmt.Printf("Couldn't establish connection to node: %s\n", err)
				return
			}
			// var GasPrice float64
			if config.GasPrice == 0 {
				config.GasPrice, err = reader.RecommendedGasPrice()
				if err != nil {
					fmt.Printf("Couldn't estimate recommended gas price: %s\n", err)
					return
				}
			}

			config.TxType, err = cmdutil.ValidTxType(reader, config.Network())
			if err != nil {
				fmt.Printf("Couldn't determine proper tx type: %s\n", err)
				return
			}

			if config.TxType == types.LegacyTxType && config.TipGas > 0 {
				fmt.Printf("We are doing legacy tx hence we ignore tip gas parameter.\n")
				return
			}

			if config.TxType == types.DynamicFeeTxType {
				if config.TipGas == 0 {
					config.TipGas, err = reader.GetSuggestedGasTipCap()
					if err != nil {
						fmt.Printf("Couldn't estimate recommended gas price: %s\n", err)
						return
					}
				}
			}

			// var GasLimit uint64
			if config.GasLimit == 0 {
				if tokenAddr == util.ETH_ADDR {
					if amountStr == "ALL" {
						config.GasLimit, err = reader.EstimateExactGas(config.From, to, 0, big.NewInt(1), []byte{})
						if err != nil {
							fmt.Printf("Getting estimated gas for the tx failed: %s\n", err)
							return
						}
						config.ExtraGasLimit = 0

						ethBalance, err := reader.GetBalance(config.From)
						if err != nil {
							fmt.Printf("Couldn't get %s balance: %s\n", config.Network().GetNativeTokenSymbol(), err)
							return
						}
						gasCost := big.NewInt(0).Mul(
							big.NewInt(int64(config.GasLimit)),
							FloatToBigInt(config.GasPrice+config.ExtraGasPrice, 9),
						)
						amountWei = big.NewInt(0).Sub(ethBalance, gasCost)
					} else {
						amountWei, err = FloatStringToBig(amountStr, config.Network().GetNativeTokenDecimal())
						if err != nil {
							fmt.Printf("Couldn't calculate send amount: %s\n", err)
							return
						}
						config.GasLimit, err = reader.EstimateExactGas(config.From, to, 0, amountWei, []byte{})
						if err != nil {
							fmt.Printf("Getting estimated gas for the tx failed: %s\n", err)
							return
						}
					}
				} else {
					var data []byte
					if amountStr == "ALL" {
						amountWei, err = reader.ERC20Balance(tokenAddr, config.From)
						if err != nil {
							fmt.Printf("Couldn't get token balance: %s\n", err)
							return
						}
						data, err = PackERC20Data(
							"transfer",
							HexToAddress(to),
							amountWei,
						)
						if err != nil {
							fmt.Printf("Couldn't pack data: %s\n", err)
							return
						}
					} else {
						decimals, err := reader.ERC20Decimal(tokenAddr)
						if err != nil {
							fmt.Printf("Couldn't get token decimal: %s\n", err)
							return
						}
						amountWei, err = FloatStringToBig(amountStr, decimals)
						if err != nil {
							fmt.Printf("Couldn't calculate token amount in wei: %s\n", err)
							return
						}

						data, err = PackERC20Data(
							"transfer",
							HexToAddress(to),
							amountWei,
						)
						if err != nil {
							fmt.Printf("Couldn't pack data: %s\n", err)
							return
						}
					}
					config.GasLimit, err = reader.EstimateGas(config.From, tokenAddr, config.GasPrice+config.ExtraGasPrice, 0, data)
					if err != nil {
						fmt.Printf("Couldn't estimate gas limit: %s\n", err)
						return
					}
				}
			}
			// var Nonce uint64
			if config.Nonce == 0 {
				config.Nonce, err = reader.GetMinedNonce(config.From)
				if err != nil {
					fmt.Printf("Couldn't get nonce: %s\n", err)
					return
				}
			}

			handleSend(
				config.TxType,
				cmd, args,
				config.GasPrice, config.ExtraGasPrice,
				config.GasLimit, config.ExtraGasLimit,
				config.Nonce,
				config.FromAcc,
				to,
				amountWei,
				tokenAddr,
			)
		},
	}

	AddCommonFlagsToTransactionalCmds(sendCmd)
	sendCmd.Flags().StringVarP(&to, "to", "t", "", "Account to send eth to. It can be ethereum address or a hint string to look it up in the address database. See jarvis addr for all of the known addresses")
	sendCmd.Flags().StringVarP(&value, "amount", "v", "0", "Amount of eth to send. It is in eth/token value, not wei/twei. If a float number is passed, it will be interpreted as ETH, otherwise, it must be in the form of `float|ALL address` or `float|ALL name`. In the later case, `name` will be used to look for the token address. Eg. 0.01, 0.01 knc, 0.01 0xdd974d5c2e2928dea5f71b9825b8b646686bd200, ALL KNC are valid values.")
	sendCmd.MarkFlagRequired("to")
	sendCmd.MarkFlagRequired("amount")

	rootCmd.AddCommand(sendCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// sendCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// sendCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
