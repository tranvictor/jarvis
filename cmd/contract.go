package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Songmu/prompter"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/txanalyzer"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/db"
	txpkg "github.com/tranvictor/jarvis/tx"
	"github.com/tranvictor/jarvis/util"
)

type Methods []abi.Method

func (self Methods) Len() int           { return len(self) }
func (self Methods) Swap(i, j int)      { self[i], self[j] = self[j], self[i] }
func (self Methods) Less(i, j int) bool { return self[i].Name < self[j].Name }

func promptTxData(contractAddress string, prefills []string) ([]byte, error) {
	analyzer := txanalyzer.NewAnalyzer()
	a, err := util.GetABI(contractAddress, Network)
	if err != nil {
		return nil, fmt.Errorf("Couldn't get the ABI: %s", err)
	}
	methods := []abi.Method{}
	for _, m := range a.Methods {
		if !m.Const {
			methods = append(methods, m)
		}
	}
	sort.Sort(Methods(methods))
	if MethodIndex == 0 {
		fmt.Printf("write functions:\n")
		for i, m := range methods {
			fmt.Printf("%d. %s\n", i+1, m.Name)
		}
		MethodIndex = uint64(promptIndex(fmt.Sprintf("Please choose method index [%d, %d]", 1, len(methods)), 1, len(methods)))
	} else if int(MethodIndex) > len(methods) {
		return nil, fmt.Errorf("The contract doesn't have %d(th) write method.")
	}
	method := methods[MethodIndex-1]
	inputs := method.Inputs
	if PrefillMode && len(inputs) != len(prefills) {
		return nil, fmt.Errorf("You must specify enough params in prefilled mode")
	}
	params := []interface{}{}
	pi := 0
	for {
		if pi >= len(inputs) {
			break
		}
		input := inputs[pi]
		var inputParam interface{}
		if !PrefillMode || prefills[pi] == "?" {
			fmt.Printf("%d. %s (%s)", pi, input.Name, input.Type.String())
			inputParam, err = promptParam(input, "")
		} else {
			inputParam, err = promptParam(input, prefills[pi])
		}
		if err != nil {
			fmt.Printf("Your input is not valid: %s\n", err)
			continue
		}
		fmt.Printf(
			"        Your input: %s\n",
			indent(8, analyzer.ParamAsString(input.Type, inputParam)),
		)
		params = append(params, inputParam)
		pi++
	}
	return a.Pack(method.Name, params...)
}

var composeDataContractCmd = &cobra.Command{
	Use:   "encode",
	Short: "Encode tx data to interact with smart contracts",
	Long: `
Param rules:
	All params are passed to this command as strings and will be treated
	with the following seperated groups:
	1. Array params
		An array param must be wrapped with [ ], spaces around the string
		will be trimmed. In side [ ], each element must separated by ","
		character. Each element will be parsed with the "type conversion"
		rule which is explained in the later section.
	2. Non array params
		A non array param will be passed as string, spaces around the
		string will be trimmed. Each element will be parsed with the
		"type conversion" rule which is explained in the later section.

	Type conversion rules:
	1. string
		string element must be wrapped by " ". The content inside " " will
		then be used as the string param.

	2. int, uint
		int and uint element can be in base 10 or 16. If it is in base 16,
		it must begin with 0x.

		further more, you can put an erc20 token symbol behind (after
		exactly 1 space) the number, Jarvis will convert it to the big
		number according to the token's decimal. In this case, the
		number can be float.

		For example:
		8.8 KNC => 8800000000000000000 (17 zeroes).
		10.5 KNC token => 10500000000000000000.

	3. bool
		bool element must be either "true" or "false" (without quotes), all
		other alternative boolean value string are invalid. Eg. T, F, True,
		False, nil are invalid.

	4. address
		address element can be either hex address or a string without " ".
		If it is a string, Jarvis will look it up in the address book
		and take the most relevant address.

	5. hash
		hash element must be represented in hex form without quotes.

	6. bytes
		bytes element must be represented in hex form without quotes. If
		0x is provided, it will be interpreted as empty bytes array.

	7. fixed length bytes
		Not supported yet
	`,
	Run: func(cmd *cobra.Command, args []string) {
		contractAddress, err := getContractFromParams(args)
		if err != nil {
			fmt.Printf("Couldn't interpret contract address")
			return
		}
		data, err := promptTxData(contractAddress, PrefillParams)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		fmt.Printf("Data to sign: 0x%s\n", common.Bytes2Hex(data))
	},
}

func getContractFromParams(args []string) (contractAddress string, err error) {
	if len(args) < 1 {
		fmt.Printf("Please specify contract address\n")
		return "", fmt.Errorf("not enough params")
	}

	addrDesc, err := db.GetAddress(args[0])
	var contractName string
	if err != nil {
		contractName = "Unknown"
		addresses := util.ScanForAddresses(args[0])
		if len(addresses) == 0 {
			fmt.Printf("Couldn't find any address for \"%s\"", args[0])
			return "", fmt.Errorf("address not found")
		}
		contractAddress = addresses[0]
	} else {
		contractName = addrDesc.Desc
		contractAddress = addrDesc.Address
	}
	fmt.Printf("Contract: %s (%s)\n", contractAddress, contractName)
	return contractAddress, nil
}

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Read, encode tx data and write contract",
	Long:  ``,
}

var txContractCmd = &cobra.Command{
	Use:              "tx",
	Short:            "do transaction to interact with smart contracts",
	Long:             ` `,
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		PrefillStr = strings.Trim(PrefillStr, " ")
		if PrefillStr != "" {
			PrefillMode = true
			PrefillParams = strings.Split(PrefillStr, "|")
			for i, _ := range PrefillParams {
				PrefillParams[i] = strings.Trim(PrefillParams[i], " ")
			}
		}

		if Value < 0 {
			return fmt.Errorf("value can't be negative")
		}

		// process from to get address
		acc, err := accounts.GetAccount(From)
		if err != nil {
			return err
		} else {
			FromAcc = acc
			From = acc.Address
		}

		To, err = getContractFromParams(args)
		if err != nil {
			return fmt.Errorf("can't interpret the contract address")
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

		// var Nonce uint64
		if Nonce == 0 {
			Nonce, err = reader.GetMinedNonce(From)
			if err != nil {
				return err
			}
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		data, err := promptTxData(To, PrefillParams)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		// var GasLimit uint64
		if GasLimit == 0 {
			GasLimit, err = reader.EstimateGas(From, To, GasPrice+ExtraGasPrice, Value, data)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}
		tx := ethutils.BuildTx(Nonce, To, Value, GasLimit, GasPrice+ExtraGasPrice, data)
		err = promptTxConfirmation(From, tx)
		if err != nil {
			fmt.Printf("Aborted!\n")
			return
		}
		fmt.Printf("== Unlock your wallet and sign now...\n")
		account, err := accounts.UnlockAccount(FromAcc, Network)
		if err != nil {
			fmt.Printf("Failed: %s\n", err)
			return
		}
		tx, broadcasted, err := account.SignTxAndBroadcast(tx)
		util.DisplayWaitAnalyze(
			tx, broadcasted, err, Network,
		)
	},
}

func showTxInfoToConfirm(from string, tx *types.Transaction) error {
	fmt.Printf("From: %s\n", util.VerboseAddress(from))
	fmt.Printf("To: %s\n", util.VerboseAddress(tx.To().Hex()))
	fmt.Printf("Value: %f ETH\n", ethutils.BigToFloat(tx.Value(), 18))
	fmt.Printf("Nonce: %d\n", tx.Nonce())
	fmt.Printf("Gas price: %f gwei\n", ethutils.BigToFloat(tx.GasPrice(), 9))
	fmt.Printf("Gas limit: %d\n", tx.Gas())
	r, err := util.EthReader(Network)
	if err != nil {
		return err
	}
	abi, err := r.GetABI(tx.To().Hex())
	if err != nil {
		return fmt.Errorf("Getting abi of the contract failed: %s", err)
	}
	analyzer := txanalyzer.NewAnalyzer()
	method, params, gnosisResult, err := analyzer.AnalyzeMethodCall(abi, tx.Data())
	if err != nil {
		return fmt.Errorf("Can't decode method call: %s", err)
	}
	fmt.Printf("Method: %s\n", method)
	for _, param := range params {
		fmt.Printf("%s: %s (%s)\n", param.Name, param.Value, param.Type)
	}
	txpkg.PrintGnosis(gnosisResult)
	return nil
}

func promptTxConfirmation(from string, tx *types.Transaction) error {
	fmt.Printf("\n------Confirm tx data before signing------\n")
	showTxInfoToConfirm(from, tx)
	if !prompter.YN("\nConfirm?", true) {
		return fmt.Errorf("Aborted!")
	}
	return nil
}

func init() {
	contractCmd.AddCommand(composeDataContractCmd)

	txContractCmd.PersistentFlags().Float64VarP(&GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	txContractCmd.PersistentFlags().Float64VarP(&ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	txContractCmd.PersistentFlags().Uint64VarP(&GasLimit, "gas", "g", 0, "Base gas limit for the tx. If default value is used, we will use ethereum nodes to estimate the gas limit. The gas limit to be used in the tx is gas limit + extra gas limit")
	txContractCmd.PersistentFlags().Uint64VarP(&ExtraGasLimit, "extragas", "G", 250000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
	txContractCmd.PersistentFlags().Uint64VarP(&Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	txContractCmd.PersistentFlags().StringVarP(&From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	txContractCmd.PersistentFlags().StringVarP(&PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	txContractCmd.PersistentFlags().Uint64VarP(&MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1.")
	txContractCmd.Flags().Float64VarP(&Value, "amount", "v", 0, "Amount of eth to send. It is in eth value, not wei.")
	txContractCmd.MarkFlagRequired("from")
	contractCmd.AddCommand(txContractCmd)
	// contractCmd.AddCommand(govInfocontractCmd)
	// TODO: add more commands to send or call other contracts
	rootCmd.AddCommand(contractCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// txCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// txCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
