package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Songmu/prompter"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/util"
)

var GasPrice float64
var ExtraGasPrice float64
var GasLimit uint64
var ExtraGasLimit uint64
var Nonce uint64
var From string
var FromAcc accounts.AccDesc

// currency here is supposed to be either ETH or address of an ERC20 token
func handleSend(
	cmd *cobra.Command, args []string,
	basePrice, extraPrice float64,
	baseGas, extraGas uint64,
	nonce uint64,
	from accounts.AccDesc,
	to string,
	amount float64,
	tokenAddr string,
) {
	fmt.Printf("== Unlock your wallet and send now...\n")
	account, err := accounts.UnlockAccount(from, Network)
	if err != nil {
		fmt.Printf("Failed: %s\n", err)
		os.Exit(126)
	}

	var (
		t           *types.Transaction
		broadcasted bool
		errors      error
	)

	fmt.Printf("token: %s, amount: %f\n", tokenAddr, amount)
	if tokenAddr == util.ETH_ADDR {
		t, broadcasted, errors = account.SendETHWithNonceAndPrice(
			Nonce, GasPrice+ExtraGasPrice,
			ethutils.FloatToBigInt(amount, 18), to,
		)
	} else {
		t, broadcasted, errors = account.SendERC20(tokenAddr, amount, to)
	}
	util.DisplayWaitAnalyze(
		t, broadcasted, errors, Network,
	)
}

func promptConfirmation(
	from accounts.AccDesc,
	to db.AddressDesc,
	nonce uint64,
	gasPrice float64,
	extraGasPrice float64,
	gasLimit uint64,
	extraGasLimit uint64,
	amount float64,
	tokenAddr string,
	tokenDesc string) error {
	fmt.Printf("From: %s - %s\n", from.Address, from.Desc)
	fmt.Printf("To: %s - %s\n", to.Address, to.Desc)
	fmt.Printf("Value: %f %s(%s)\n", amount, tokenDesc, tokenAddr)
	fmt.Printf("Nonce: %d\n", nonce)
	fmt.Printf("Gas price: %f gwei\n", gasPrice+extraGasPrice)
	fmt.Printf("Gas limit: %d\n", gasLimit+extraGasLimit)
	if !prompter.YN("Confirm?", true) {
		return fmt.Errorf("Aborted!")
	}
	return nil
}

func init() {
	var to string
	var amount float64
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

			amount, currency, err = util.ValueToAmountAndCurrency(value)
			if err != nil {
				return err
			}

			// if value is not an address, we need to look it up
			// from the token database to get its address

			if currency == util.ETH_ADDR || strings.ToLower(currency) == "eth" {
				tokenAddr = util.ETH_ADDR
				tokenDesc = "ETH"
			} else {
				addrDesc, err := db.GetTokenAddress(currency)
				if err != nil {
					if util.IsAddress(currency) {
						tokenAddr = currency
						tokenDesc = "unrecognized"
					} else {
						return err
					}
				} else {
					tokenAddr = addrDesc.Address
					tokenDesc = addrDesc.Desc
				}
			}
			// process from to get address
			acc, err := accounts.GetAccount(From)
			if err != nil {
				return err
			} else {
				FromAcc = acc
				From = acc.Address
			}
			// process to to get address
			addr, err := db.GetAddress(to)
			if err != nil {
				return err
			} else {
				to = addr.Address
			}
			fmt.Printf("Network: %s\n", Network)
			reader, err := util.EthReader(Network)
			if err != nil {
				return err
			}
			// var GasPrice float64
			if GasPrice == 0 {
				GasPrice, err = reader.RecommendedGasPrice()
				if err != nil {
					return err
				}
			}
			// var GasLimit uint64
			if GasLimit == 0 {
				if tokenAddr == util.ETH_ADDR {
					GasLimit, err = reader.EstimateGas(From, to, GasPrice+ExtraGasPrice, amount, []byte{})
					if err != nil {
						return err
					}
				} else {
					decimals, err := reader.ERC20Decimal(tokenAddr)
					if err != nil {
						return err
					}
					data, err := ethutils.PackERC20Data(
						"transfer",
						ethutils.HexToAddress(to),
						ethutils.FloatToBigInt(amount, decimals),
					)
					if err != nil {
						return err
					}
					GasLimit, err = reader.EstimateGas(From, tokenAddr, GasPrice+ExtraGasPrice, 0, data)
					if err != nil {
						return err
					}
				}
			}
			// var Nonce uint64
			if Nonce == 0 {
				Nonce, err = reader.GetMinedNonce(From)
				if err != nil {
					return err
				}
			}
			err = promptConfirmation(
				acc,
				addr,
				Nonce,
				GasPrice,
				ExtraGasPrice,
				GasLimit,
				ExtraGasLimit,
				amount,
				tokenAddr,
				tokenDesc,
			)
			if err != nil {
				os.Exit(126)
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			handleSend(
				cmd, args,
				GasPrice, ExtraGasPrice,
				GasLimit, ExtraGasLimit,
				Nonce,
				FromAcc,
				to,
				amount,
				tokenAddr,
			)
		},
	}

	sendCmd.PersistentFlags().Float64VarP(&GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	sendCmd.PersistentFlags().Float64VarP(&ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	sendCmd.PersistentFlags().Uint64VarP(&GasLimit, "gas", "g", 0, "Base gas limit for the tx. If default value is used, we will use ethereum nodes to estimate the gas limit. The gas limit to be used in the tx is gas limit + extra gas limit")
	// sendCmd.PersistentFlags().Uint64VarP(&ExtraGasLimit, "extragas", "G", 250000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
	sendCmd.PersistentFlags().Uint64VarP(&Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	sendCmd.PersistentFlags().StringVarP(&From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	sendCmd.Flags().StringVarP(&to, "to", "t", "", "Account to send eth to. It can be ethereum address or a hint string to look it up in the address database. See jarvis addr for all of the known addresses")
	sendCmd.Flags().StringVarP(&value, "amount", "v", "0", "Amount of eth to send. It is in eth/token value, not wei/twei. If a float number is passed, it will be interpreted as ETH, otherwise, it must be in the form of `float address` or `float name`. In the later case, `name` will be used to look for the token address. Eg. 0.01, 0.01 knc, 0.01 0xdd974d5c2e2928dea5f71b9825b8b646686bd200 are valid values.")
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
