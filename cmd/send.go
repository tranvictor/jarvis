package cmd

import (
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

// currency here is supposed to be either ETH or address of an ERC20 token
func handleSend(
	cmd *cobra.Command, args []string,
	basePrice, extraPrice float64,
	baseGas, extraGas uint64,
	nonce uint64,
	from accounts.AccDesc,
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
		t = ethutils.BuildExactTx(
			config.Nonce,
			to,
			amountWei,
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			[]byte{},
		)
	} else {
		a, err = ethutils.GetERC20ABI()
		if err != nil {
			fmt.Printf("Couldn't get erc20 abi: %s\n", err)
			return
		}
		data, err := a.Pack(
			"transfer",
			ethutils.HexToAddress(to),
			amountWei,
		)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		t = ethutils.BuildExactTx(
			config.Nonce,
			tokenAddr,
			big.NewInt(0),
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			data,
		)
	}

	err = util.PromptTxConfirmation(
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
	account, err := accounts.UnlockAccount(from, config.Network())
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		os.Exit(126)
	}

	if config.DontBroadcast {
		signedTx, err := account.SignTx(t)
		if err != nil {
			fmt.Printf("%s", err)
			return
		}
		data, err := rlp.EncodeToBytes(signedTx)
		if err != nil {
			fmt.Printf("Couldn't encode the signed tx: %s", err)
			return
		}
		fmt.Printf("Signed tx: %s\n", hexutil.Encode(data))
	} else {
		tx, broadcasted, err := account.SignTxAndBroadcast(t)
		if config.DontWaitToBeMined {
			util.DisplayBroadcastedTx(
				tx, broadcasted, err, config.Network(),
			)
		} else {
			util.DisplayWaitAnalyze(
				reader, analyzer, tx, broadcasted, err, config.Network(),
				a, nil,
			)
		}
	}
}

func init() {
	var to string
	var amountStr string
	var amountWei *big.Int
	var value string
	var tokenAddr string
	var tokenDesc string
	var currency string
	var err error
	// sendCmd represents the send command
	var sendCmd = &cobra.Command{
		Use:   "send",
		Short: "Send eth or erc20 token from your account to others",
		Long: `Send eth or erc20 token from your account to other accounts.
The token and accounts can be specified either by memorable name or
exact addresses start with 0x.`,
		TraverseChildren: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

			amountStr, currency, err = util.ValueToAmountAndCurrency(value)
			if err != nil {
				return err
			}

			// if value is not an address, we need to look it up
			// from the token database to get its address

			if currency == util.ETH_ADDR || strings.ToLower(currency) == "eth" {
				tokenAddr = util.ETH_ADDR
				tokenDesc = "ETH"
			} else {
				addr, name, err := util.GetMatchingAddress(fmt.Sprintf("%s token", currency))
				if err != nil {
					if util.IsAddress(currency) {
						tokenAddr = currency
						tokenDesc = "unrecognized"
					} else {
						return err
					}
				} else {
					tokenAddr = addr
					tokenDesc = name
				}
			}

			// process from to get address
			acc, err := accounts.GetAccount(config.From)
			if err != nil {
				return err
			} else {
				config.FromAcc = acc
				config.From = acc.Address
			}
			// process to to get address
			toAddr, _, err := util.GetMatchingAddress(to)
			if err != nil {
				return err
			} else {
				to = toAddr
			}
			reader, err := util.EthReader(config.Network())
			if err != nil {
				return err
			}
			// var GasPrice float64
			if config.GasPrice == 0 {
				config.GasPrice, err = reader.RecommendedGasPrice()
				if err != nil {
					return err
				}
			}
			// var GasLimit uint64
			if config.GasLimit == 0 {
				if tokenAddr == util.ETH_ADDR {
					if amountStr == "ALL" {
						config.GasLimit, err = reader.EstimateExactGas(config.From, to, config.GasPrice+config.ExtraGasPrice, big.NewInt(1), []byte{})
						if err != nil {
							fmt.Printf("Getting estimated gas for the tx failed: %s\n", err)
							return err
						}
						ethBalance, err := reader.GetBalance(config.From)
						if err != nil {
							return err
						}
						gasCost := big.NewInt(0).Mul(
							big.NewInt(int64(config.GasLimit)),
							ethutils.FloatToBigInt(config.GasPrice+config.ExtraGasPrice, 9),
						)
						amountWei = big.NewInt(0).Sub(ethBalance, gasCost)
					} else {
						amountWei, err = util.FloatStringToBig(amountStr, config.Network().GetNativeTokenDecimal())
						if err != nil {
							return err
						}
						config.GasLimit, err = reader.EstimateExactGas(config.From, to, config.GasPrice+config.ExtraGasPrice, amountWei, []byte{})
						if err != nil {
							fmt.Printf("Getting estimated gas for the tx failed: %s\n", err)
							return err
						}
					}
				} else {
					var data []byte

					if amountStr == "ALL" {
						amountWei, err = reader.ERC20Balance(tokenAddr, config.From)
						if err != nil {
							return err
						}
						data, err = ethutils.PackERC20Data(
							"transfer",
							ethutils.HexToAddress(to),
							amountWei,
						)
						if err != nil {
							return err
						}
					} else {
						decimals, err := reader.ERC20Decimal(tokenAddr)
						if err != nil {
							return err
						}
						amountWei, err = util.FloatStringToBig(amountStr, decimals)
						if err != nil {
							return err
						}

						data, err = ethutils.PackERC20Data(
							"transfer",
							ethutils.HexToAddress(to),
							amountWei,
						)
						if err != nil {
							return err
						}
					}
					config.GasLimit, err = reader.EstimateGas(config.From, tokenAddr, config.GasPrice+config.ExtraGasPrice, 0, data)
					if err != nil {
						return err
					}
				}
			}
			// var Nonce uint64
			if config.Nonce == 0 {
				config.Nonce, err = reader.GetMinedNonce(config.From)
				if err != nil {
					return err
				}
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			handleSend(
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

	sendCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	sendCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	sendCmd.PersistentFlags().Uint64VarP(&config.GasLimit, "gas", "g", 0, "Base gas limit for the tx. If default value is used, we will use ethereum nodes to estimate the gas limit. The gas limit to be used in the tx is gas limit + extra gas limit")
	// sendCmd.PersistentFlags().Uint64VarP(&ExtraGasLimit, "extragas", "G", 250000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
	sendCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	sendCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	sendCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	sendCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
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
