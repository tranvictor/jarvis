package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/accounts"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

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

		reader, err := util.EthReader(config.Network())
		if err != nil {
			fmt.Printf("Couldn't connect to blockchain.\n")
			return
		}

		analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

		a, err := util.ConfigToABI(contractAddress, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			fmt.Printf("Couldn't get abi for %s: %s\n", contractAddress, err)
			return
		}

		data, err := cmdutil.PromptTxData(
			analyzer,
			contractAddress,
			config.MethodIndex,
			config.PrefillParams,
			config.PrefillMode,
			a,
			nil,
			config.Network(),
		)
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
	PersistentPreRunE: cmdutil.CommonTxPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network())
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}

		analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

		a, err := util.ConfigToABI(config.To, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			fmt.Printf("Couldn't get abi for %s: %s\n", config.To, err)
			return
		}

		data, err := cmdutil.PromptTxData(
			analyzer,
			config.To,
			config.MethodIndex,
			config.PrefillParams,
			config.PrefillMode,
			a,
			nil,
			config.Network(),
		)

		jarviscommon.DebugPrintf("calling data: %x\n", data)

		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		// var GasLimit uint64
		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateExactGas(config.From, config.To, 0, config.Value, data)
			if err != nil {
				fmt.Printf("Couldn't estimate gas limit: %s\n", err)
				return
			}
		}

		tx := jarviscommon.BuildExactTx(
			config.TxType,
			config.Nonce,
			config.To,
			config.Value,
			config.GasLimit+config.ExtraGasLimit,
			config.GasPrice+config.ExtraGasPrice,
			config.TipGas+config.ExtraTipGas,
			data,
			config.Network().GetChainID(),
		)
		err = cmdutil.PromptTxConfirmation(
			analyzer,
			util.GetJarvisAddress(config.From, config.Network()),
			tx,
			map[string]*abi.ABI{
				strings.ToLower(config.To): a,
			},
			config.Network(),
		)
		if err != nil {
			fmt.Printf("Aborted!\n")
			return
		}
		fmt.Printf("== Unlock your wallet and sign now...\n")
		account, err := accounts.UnlockAccount(config.FromAcc)
		if err != nil {
			fmt.Printf("Unlock your wallet failed: %s\n", err)
			return
		}

		signedAddr, signedTx, err := account.SignTx(tx, big.NewInt(int64(config.Network().GetChainID())))
		if err != nil {
			fmt.Printf("%s", err)
			return
		}
		if signedAddr.Cmp(jarviscommon.HexToAddress(config.FromAcc.Address)) != 0 {
			fmt.Printf("Signed from wrong address. You could use wrong hw or passphrase. Expected wallet: %s, signed wallet: %s\n",
				config.FromAcc.Address,
				signedAddr.Hex(),
			)
			return
		}

		broadcasted, err := cmdutil.HandlePostSign(signedTx, reader, analyzer, a)
		if err != nil && !broadcasted {
			fmt.Printf("Failed to proceed after signing the tx: %s. Aborted.\n", err)
		}
	},
}

func handleReadOneFunctionOnContract(reader *reader.EthReader, a *abi.ABI, atBlock int64, method *abi.Method, params []interface{}) (contractReadResult, error) {
	responseBytes, err := reader.ReadContractToBytes(atBlock, "0x0000000000000000000000000000000000000000", config.To, a, method.Name, params...)
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

	analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

	fmt.Printf("Output:\n")
	result := contractReadResult{}
	for i, output := range method.Outputs {
		oneOutputParamResult := analyzer.ParamAsJarvisParamResult(output.Name, output.Type, ps[i])
		oneVerboseParamResult := convertToVerboseParamResult(oneOutputParamResult)
		result = append(result, oneVerboseParamResult)

		// 	returnVariable{
		// 	Name:        output.Name,
		// 	Values:      valueStrs,
		// 	HumanValues: VerboseValues(values),
		// })

		fmt.Printf("%d. ", i+1)
		jarviscommon.PrintVerboseParamResultToWriter(os.Stdout, oneOutputParamResult, 0, true)
	}
	return result, nil
}

func convertToVerboseParamResult(oneOutputParamResult jarviscommon.ParamResult) verboseParamResult {
	result := verboseParamResult{
		Name: oneOutputParamResult.Name,
		Type: oneOutputParamResult.Type,
	}

	if oneOutputParamResult.Values != nil {
		result.Values = []string{}
		for _, v := range oneOutputParamResult.Values {
			result.Values = append(result.Values, v.Value)
		}
		result.HumanValues = jarviscommon.VerboseValues(oneOutputParamResult.Values)
	}

	if oneOutputParamResult.Tuples != nil {
		result.Tuples = []verboseTupleParamResult{}
		for _, t := range oneOutputParamResult.Tuples {
			result.Tuples = append(result.Tuples, convertToVerboseTupleParamResult(t))
		}
	}

	if oneOutputParamResult.Arrays != nil {
		result.Arrays = []verboseParamResult{}
		for _, a := range oneOutputParamResult.Arrays {
			result.Arrays = append(result.Arrays, convertToVerboseParamResult(a))
		}
	}

	return result
}

func convertToVerboseTupleParamResult(oneTupleParamResult jarviscommon.TupleParamResult) verboseTupleParamResult {
	result := verboseTupleParamResult{
		Name: oneTupleParamResult.Name,
		Type: oneTupleParamResult.Type,
	}

	if oneTupleParamResult.Values != nil {
		result.Values = []verboseParamResult{}
		for _, v := range oneTupleParamResult.Values {
			result.Values = append(result.Values, convertToVerboseParamResult(v))
		}
	}

	return result
}

type verboseParamResult struct {
	Name        string                    `json:"name"`
	Type        string                    `json:"type"`
	Values      []string                  `json:"values"`
	Tuples      []verboseTupleParamResult `json:"tuples"`
	Arrays      []verboseParamResult      `json:"arrays"`
	HumanValues []string                  `json:"human_values"`
}

type verboseTupleParamResult struct {
	Name   string               `json:"name"`
	Type   string               `json:"type"`
	Values []verboseParamResult `json:"values"`
}

type contractReadResult []verboseParamResult

type contractReadResultJSON struct {
	Result contractReadResult `json:"result"`
	Error  string             `json:"error"`
}

func (c *contractReadResultJSON) Write(filepath string) {
	data, _ := json.MarshalIndent(c, "", "  ")
	err := ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		fmt.Printf("Writing to json file failed: %s\n", err)
	}
}

type batchcontractReadResultJSON struct {
	Functions []string                 `json:"functions"`
	Results   []contractReadResultJSON `json:"results"`
}

func (b *batchcontractReadResultJSON) Write(filepath string) {
	data, _ := json.MarshalIndent(b, "", "  ")
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
	PersistentPreRunE: cmdutil.CommonFunctionCallPreprocess,
	Run: func(cmd *cobra.Command, args []string) {
		reader, err := util.EthReader(config.Network())
		if err != nil {
			fmt.Printf("Couldn't init eth reader: %s\n", err)
			return
		}
		if config.AllZeroParamsMethods {
			resultJSON := batchcontractReadResultJSON{
				Functions: []string{},
				Results:   []contractReadResultJSON{},
			}

			if config.JSONOutputFile != "" {
				defer resultJSON.Write(config.JSONOutputFile)
			}

			a, err := util.ConfigToABI(config.To, config.ForceERC20ABI, config.CustomABI, config.Network())
			if err != nil {
				fmt.Printf("Couldn't get abi for %s: %s\n", config.To, err)
				return
			}

			methods := cmdutil.AllZeroParamFunctions(a)
			for i := range methods {
				method := methods[i]
				resultJSON.Functions = append(resultJSON.Functions, method.Name)
				fmt.Printf("%d. %s\n", i+1, method.Name)

				result, err := handleReadOneFunctionOnContract(reader, a, config.AtBlock, &method, []interface{}{})
				if err != nil {
					resultJSON.Results = append(resultJSON.Results, contractReadResultJSON{
						Error: fmt.Sprintf("%s", err),
					})
				} else {
					resultJSON.Results = append(resultJSON.Results, contractReadResultJSON{
						Result: result,
					})
				}
				fmt.Printf("---------------------------------------------------\n")
			}
		} else {
			resultJSON := contractReadResultJSON{
				Result: contractReadResult{},
				Error:  "",
			}

			if config.JSONOutputFile != "" {
				defer resultJSON.Write(config.JSONOutputFile)
			}

			analyzer := txanalyzer.NewGenericAnalyzer(reader, config.Network())

			a, err := util.ConfigToABI(config.To, config.ForceERC20ABI, config.CustomABI, config.Network())
			if err != nil {
				fmt.Printf("Couldn't get abi for %s: %s\n", config.To, err)
				return
			}

			method, params, err := cmdutil.PromptFunctionCallData(
				analyzer,
				config.To,
				config.MethodIndex,
				config.PrefillParams,
				config.PrefillMode,
				"read",
				a,
				nil,
				config.Network(),
			)
			if err != nil {
				fmt.Printf("Couldn't get params from users: %s\n", err)
				resultJSON.Error = fmt.Sprintf("%s", err)
				return
			}
			result, err := handleReadOneFunctionOnContract(reader, a, config.AtBlock, method, params)

			if err != nil {
				resultJSON.Error = fmt.Sprintf("%s", err)
			} else {
				resultJSON.Result = result
			}
		}
	},
}

func init() {
	contractCmd.AddCommand(composeDataContractCmd)

	AddCommonFlagsToTransactionalCmds(txContractCmd)
	txContractCmd.PersistentFlags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	txContractCmd.PersistentFlags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1.")
	txContractCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	txContractCmd.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
	txContractCmd.Flags().StringVarP(&config.RawValue, "amount", "v", "0", "Amount of eth to send. It is in eth value, not wei.")
	txContractCmd.MarkFlagRequired("from")
	contractCmd.AddCommand(txContractCmd)

	readContractCmd.PersistentFlags().StringVarP(&config.PrefillStr, "prefills", "I", "", "Prefill params string. Each param is separated by | char. If the param is \"?\", user input will be prompted.")
	readContractCmd.PersistentFlags().Uint64VarP(&config.MethodIndex, "method-index", "M", 0, "Index of the method in alphabeth sorted method list of the contract. Index counts from 1. This param will be IGNORED if -a or --all is true.")
	readContractCmd.PersistentFlags().BoolVarP(&config.AllZeroParamsMethods, "all", "a", false, "Read all functions that don't have any params")
	readContractCmd.PersistentFlags().BoolVarP(&config.ForceERC20ABI, "erc20-abi", "e", false, "Use ERC20 ABI where possible.")
	readContractCmd.PersistentFlags().Int64VarP(&config.AtBlock, "block", "b", -1, "Specify the block to read at. Default value indicates reading at latest state of the chain.")
	readContractCmd.PersistentFlags().StringVarP(&config.JSONOutputFile, "json-output", "o", "", "write output of contract read to json file")
	readContractCmd.PersistentFlags().StringVarP(&config.CustomABI, "abi", "c", "", "Custom abi. It can be either an address, a path to an abi file or an url to an abi. If it is an address, the abi of that address from etherscan will be queried. This param only takes effect if erc20-abi param is not true.")
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
