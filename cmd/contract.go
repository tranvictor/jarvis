package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"

	"github.com/Songmu/prompter"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
	"github.com/tranvictor/jarvis/accounts"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
)

type Methods []abi.Method

func (self Methods) Len() int           { return len(self) }
func (self Methods) Swap(i, j int)      { self[i], self[j] = self[j], self[i] }
func (self Methods) Less(i, j int) bool { return self[i].Name < self[j].Name }

func promptFunctionCallData(contractAddress string, prefills []string, mode string, forceERC20ABI bool) (*abi.ABI, *abi.Method, []interface{}, error) {
	analyzer, err := util.EthAnalyzer(config.Network)
	if err != nil {
		return nil, nil, nil, err
	}
	var a *abi.ABI
	if forceERC20ABI {
		a, err = ethutils.GetERC20ABI()
	} else {
		a, err = util.GetABI(contractAddress, config.Network)
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Couldn't get the ABI: %s", err)
	}
	methods := []abi.Method{}
	if mode == "write" {
		for _, m := range a.Methods {
			if !m.IsConstant() {
				methods = append(methods, m)
			}
		}
	} else {
		for _, m := range a.Methods {
			if m.IsConstant() {
				methods = append(methods, m)
			}
		}
	}
	sort.Sort(Methods(methods))
	if config.MethodIndex == 0 {
		fmt.Printf("write functions:\n")
		for i, m := range methods {
			fmt.Printf("%d. %s\n", i+1, m.Name)
		}
		config.MethodIndex = uint64(util.PromptIndex(fmt.Sprintf("Please choose method index [%d, %d]", 1, len(methods)), 1, len(methods)))
	} else if int(config.MethodIndex) > len(methods) {
		return nil, nil, nil, fmt.Errorf("The contract doesn't have %d(th) write method.", config.MethodIndex)
	}
	method := methods[config.MethodIndex-1]
	fmt.Printf("\nMethod: %s\n", method.Name)
	inputs := method.Inputs
	if config.PrefillMode && len(inputs) != len(prefills) {
		return nil, nil, nil, fmt.Errorf("You must specify enough params in prefilled mode")
	}
	fmt.Printf("Input:\n")
	params := []interface{}{}
	pi := 0
	for {
		if pi >= len(inputs) {
			break
		}
		input := inputs[pi]
		var inputParam interface{}
		if !config.PrefillMode || prefills[pi] == "?" {
			fmt.Printf("%d. %s (%s)", pi, input.Name, input.Type.String())
			inputParam, err = util.PromptParam(input, "", config.Network)
		} else {
			inputParam, err = util.PromptParam(input, prefills[pi], config.Network)
		}
		if err != nil {
			fmt.Printf("Your input is not valid: %s\n", err)
			continue
		}
		fmt.Printf(
			"    You entered: %s\n",
			indent(8, util.VerboseValues(analyzer.ParamAsStrings(input.Type, inputParam), config.Network)),
		)
		params = append(params, inputParam)
		pi++
	}
	return a, &method, params, nil
}

func promptTxData(contractAddress string, prefills []string, forceERC20ABI bool) ([]byte, error) {
	a, method, params, err := promptFunctionCallData(contractAddress, prefills, "write", forceERC20ABI)
	if err != nil {
		return []byte{}, err
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
		if len(args) < 1 {
			fmt.Printf("Not enough param. Please provide contract address or its name.\n")
			return
		}
		contractAddress, contractName, err := util.GetAddressFromString(args[0])
		if err != nil {
			fmt.Printf("Couldn't interpret contract address")
			return
		}
		fmt.Printf("Contract: %s (%s)\n", contractAddress, contractName)
		data, err := promptTxData(contractAddress, config.PrefillParams, config.ForceERC20ABI)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		fmt.Printf("Data to sign: 0x%s\n", common.Bytes2Hex(data))
	},
}

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Read, encode tx data and write contract",
	Long:  ``,
}

var txContractCmd = &cobra.Command{
	Use:               "tx [tx hashes]",
	Aliases:           []string{"write"},
	Short:             "do transaction to interact with smart contracts",
	Long:              ` `,
	TraverseChildren:  true,
	PersistentPreRunE: CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		data, err := promptTxData(config.To, config.PrefillParams, config.ForceERC20ABI)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		// var GasLimit uint64
		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateGas(config.From, config.To, config.GasPrice+config.ExtraGasPrice, config.Value, data)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}

		tx := ethutils.BuildTx(config.Nonce, config.To, config.Value, config.GasLimit+config.ExtraGasLimit, config.GasPrice+config.ExtraGasPrice, data)
		err = promptTxConfirmation(config.From, tx)
		if err != nil {
			fmt.Printf("Aborted!\n")
			return
		}
		fmt.Printf("== Unlock your wallet and sign now...\n")
		account, err := accounts.UnlockAccount(config.FromAcc, config.Network)
		if err != nil {
			fmt.Printf("Unlock your wallet failed: %s\n", err)
			return
		}

		if config.DontBroadcast {
			signedTx, err := account.SignTx(tx)
			if err != nil {
				fmt.Printf("%s", err)
				return
			}
			data, err := rlp.EncodeToBytes(signedTx)
			if err != nil {
				fmt.Printf("Couldn't encode the signed tx: %s", err)
				return
			}
			fmt.Printf("Signed tx: %s\n", common.ToHex(data))
		} else {
			tx, broadcasted, err := account.SignTxAndBroadcast(tx)
			if config.DontWaitToBeMined {
				util.DisplayBroadcastedTx(
					tx, broadcasted, err, config.Network,
				)
			} else {
				util.DisplayWaitAnalyze(
					tx, broadcasted, err, config.Network,
				)
			}
		}
	},
}

func allZeroParamFunctions(contractAddress string) (*abi.ABI, []abi.Method, error) {
	var a *abi.ABI
	var err error
	if config.ForceERC20ABI {
		a, err = ethutils.GetERC20ABI()
	} else {
		a, err = util.GetABI(contractAddress, config.Network)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("Couldn't get the ABI: %s", err)
	}
	methods := []abi.Method{}
	for _, m := range a.Methods {
		if m.IsConstant() && len(m.Inputs) == 0 {
			methods = append(methods, m)
		}
	}
	sort.Sort(Methods(methods))
	return a, methods, nil
}

func handleReadOneFunctionOnContract(reader *reader.EthReader, a *abi.ABI, atBlock int64, method *abi.Method, params []interface{}) (contractReadResult, error) {
	responseBytes, err := reader.ReadContractToBytes(atBlock, config.To, a, method.Name, params...)
	if err != nil {
		fmt.Printf("getting response failed: %s\n", err)
		return contractReadResult{}, err
	}
	if len(responseBytes) == 0 {
		fmt.Printf("the function reverts. please double check your params.\n")
		return contractReadResult{}, fmt.Errorf("the function reverted")
	}
	ps, err := method.Outputs.UnpackValues(responseBytes)
	if err != nil {
		fmt.Printf("Couldn't unpack response to go types: %s\n", err)
		return contractReadResult{}, err
	}
	analyzer, err := util.EthAnalyzer(config.Network)
	if err != nil {
		fmt.Printf("Couldn't init analyzer: %s\n", err)
		return contractReadResult{}, err
	}
	fmt.Printf("Output:\n")
	result := contractReadResult{}
	for i, output := range method.Outputs {
		result = append(result, returnVariable{
			Name:        output.Name,
			Values:      analyzer.ParamAsStrings(output.Type, ps[i]),
			HumanValues: util.VerboseValues(analyzer.ParamAsStrings(output.Type, ps[i]), config.Network),
		})

		fmt.Printf(
			"%d. %s (%s): %s\n",
			i+1,
			output.Name,
			output.Type.String(),
			indent(8, util.VerboseValues(analyzer.ParamAsStrings(output.Type, ps[i]), config.Network)),
		)
	}
	return result, nil
}

type returnVariable struct {
	Name        string   `json:"name"`
	Values      []string `json:"values"`
	HumanValues []string `json:"human_values"`
}

type contractReadResult []returnVariable

type contractReadResultJson struct {
	Result contractReadResult `json:"result"`
	Error  string             `json:"error"`
}

func (self *contractReadResultJson) Write(filepath string) {
	data, _ := json.MarshalIndent(self, "", "  ")
	err := ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		fmt.Printf("Writing to json file failed: %s\n", err)
	}
}

type batchContractReadResultJson struct {
	Functions []string                 `json:"functions"`
	Results   []contractReadResultJson `json:"results"`
}

func (self *batchContractReadResultJson) Write(filepath string) {
	data, _ := json.MarshalIndent(self, "", "  ")
	err := ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		fmt.Printf("Writing to json file failed: %s\n", err)
	}
}

var readContractCmd = &cobra.Command{
	Use:               "read",
	Short:             "read smart contracts (faster than etherscan)",
	Long:              ` `,
	TraverseChildren:  true,
	PersistentPreRunE: CommonFunctionCallPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network)
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		if config.AllZeroParamsMethods {
			resultJson := batchContractReadResultJson{
				Functions: []string{},
				Results:   []contractReadResultJson{},
			}

			if config.JSONOutputFile != "" {
				defer resultJson.Write(config.JSONOutputFile)
			}

			a, methods, err := allZeroParamFunctions(config.To)
			if err != nil {
				fmt.Printf("Couldn't get all zero param functions of the contract: %s\n", err)
				return
			}
			for i, _ := range methods {
				method := methods[i]
				resultJson.Functions = append(resultJson.Functions, method.Name)
				fmt.Printf("%d. %s\n", i+1, method.Name)

				result, err := handleReadOneFunctionOnContract(reader, a, config.AtBlock, &method, []interface{}{})
				if err != nil {
					resultJson.Results = append(resultJson.Results, contractReadResultJson{
						Error: fmt.Sprintf("%s", err),
					})
				} else {
					resultJson.Results = append(resultJson.Results, contractReadResultJson{
						Result: result,
					})
				}
				fmt.Printf("---------------------------------------------------\n")
			}
		} else {
			resultJson := contractReadResultJson{
				Result: contractReadResult{},
				Error:  "",
			}

			if config.JSONOutputFile != "" {
				defer resultJson.Write(config.JSONOutputFile)
			}

			a, method, params, err := promptFunctionCallData(config.To, config.PrefillParams, "read", config.ForceERC20ABI)
			if err != nil {
				fmt.Printf("Couldn't get params from users: %s\n", err)
				resultJson.Error = fmt.Sprintf("%s", err)
				return
			}
			result, err := handleReadOneFunctionOnContract(reader, a, config.AtBlock, method, params)

			if err != nil {
				resultJson.Error = fmt.Sprintf("%s", err)
			} else {
				resultJson.Result = result
			}
		}
	},
}

func showTxInfoToConfirm(from string, tx *types.Transaction) error {
	fmt.Printf("From: %s\n", util.VerboseAddress(from, config.Network))
	fmt.Printf("To: %s\n", util.VerboseAddress(tx.To().Hex(), config.Network))
	fmt.Printf("Value: %f ETH\n", ethutils.BigToFloat(tx.Value(), 18))
	fmt.Printf("Nonce: %d\n", tx.Nonce())
	fmt.Printf("Gas price: %f gwei\n", ethutils.BigToFloat(tx.GasPrice(), 9))
	fmt.Printf("Gas limit: %d\n", tx.Gas())
	var a *abi.ABI
	var err error
	if config.ForceERC20ABI {
		a, err = ethutils.GetERC20ABI()
	} else {
		a, err = util.GetABI(tx.To().Hex(), config.Network)
	}
	if err != nil {
		return fmt.Errorf("Getting abi of the contract failed: %s", err)
	}
	analyzer := txanalyzer.NewAnalyzer()
	method, params, gnosisResult, err := analyzer.AnalyzeMethodCall(a, tx.Data())
	if err != nil {
		return fmt.Errorf("Can't decode method call: %s", err)
	}
	fmt.Printf("\nContract: %s\n", util.VerboseAddress(tx.To().Hex(), config.Network))
	fmt.Printf("Method: %s\n", method)
	for _, param := range params {
		fmt.Printf(
			" . %s (%s): %s\n",
			param.Name,
			param.Type,
			util.DisplayValues(param.Value, config.Network),
		)
	}
	util.PrintGnosis(gnosisResult)
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

	txContractCmd.PersistentFlags().Float64VarP(&config.GasPrice, "gasprice", "p", 0, "Gas price in gwei. If default value is used, we will use https://ethgasstation.info/ to get fast gas price. The gas price to be used in the tx is gas price + extra gas price")
	txContractCmd.PersistentFlags().Float64VarP(&config.ExtraGasPrice, "extraprice", "P", 0, "Extra gas price in gwei. The gas price to be used in the tx is gas price + extra gas price")
	txContractCmd.PersistentFlags().Uint64VarP(&config.GasLimit, "gas", "g", 0, "Base gas limit for the tx. If default value is used, we will use ethereum nodes to estimate the gas limit. The gas limit to be used in the tx is gas limit + extra gas limit")
	txContractCmd.PersistentFlags().Uint64VarP(&config.ExtraGasLimit, "extragas", "G", 250000, "Extra gas limit for the tx. The gas limit to be used in the tx is gas limit + extra gas limit")
	txContractCmd.PersistentFlags().Uint64VarP(&config.Nonce, "nonce", "n", 0, "Nonce of the from account. If default value is used, we will use the next available nonce of from account")
	txContractCmd.PersistentFlags().StringVarP(&config.From, "from", "f", "", "Account to use to send the transaction. It can be ethereum address or a hint string to look it up in the list of account. See jarvis acc for all of the registered accounts")
	txContractCmd.PersistentFlags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	txContractCmd.PersistentFlags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1.")
	txContractCmd.PersistentFlags().BoolVarP(&config.DontBroadcast, "dry", "d", false, "Will not broadcast the tx, only show signed tx.")
	txContractCmd.PersistentFlags().BoolVarP(&config.DontWaitToBeMined, "no-wait", "F", false, "Will not wait the tx to be mined.")
	txContractCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	txContractCmd.Flags().Float64VarP(&config.Value, "amount", "v", 0, "Amount of eth to send. It is in eth value, not wei.")
	txContractCmd.MarkFlagRequired("from")
	contractCmd.AddCommand(txContractCmd)

	readContractCmd.PersistentFlags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	readContractCmd.PersistentFlags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1. This param will be IGNORED if -a or --all is true.")
	readContractCmd.PersistentFlags().BoolVarP(&config.AllZeroParamsMethods, "all", "a", false, "Read all functions that don't have any params")
	readContractCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	readContractCmd.PersistentFlags().Int64VarP(&config.AtBlock, "block", "b", -1, "Specify the block to read at. Default value indicates reading at latest state of the chain.")
	readContractCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "write output of contract read to json file")
	contractCmd.AddCommand(readContractCmd)
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
