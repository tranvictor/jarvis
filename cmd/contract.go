package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/util"
	readerPkg "github.com/tranvictor/jarvis/util/reader"
)

var composeDataContractCmd = &cobra.Command{
	Use:   "encode",
	Short: "Encode tx data to interact with smart contracts",
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonFunctionCallPreprocess(appUI, cmd, args)
	},
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
		bool element must be either "true" or "false" ( without quotes), all
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
		tc, _ := cmdutil.TxContextFrom(cmd)

		contractAddress, contractName, err := tc.Resolver.GetAddressFromString(args[0])
		if err != nil {
			appUI.Error("Couldn't interpret contract address")
			return
		}
		appUI.Info("Contract: %s (%s)", contractAddress, contractName)

		a, err := tc.Resolver.ConfigToABI(contractAddress, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			appUI.Error("Couldn't get abi for %s: %s", contractAddress, err)
			return
		}

		data, err := cmdutil.PromptTxData(
			appUI,
			tc.Analyzer,
			contractAddress,
			config.MethodIndex,
			tc.PrefillParams,
			tc.PrefillMode,
			a,
			nil,
			config.Network(),
		)
		if err != nil {
			appUI.Error("Couldn't pack data: %s", err)
			return
		}
		appUI.Success("Data to sign: 0x%s", common.Bytes2Hex(data))
	},
}

var contractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Read, encode tx data and write contract",
	Long:  ``,
}

var txContractCmd = &cobra.Command{
	Use:              "tx [tx hashes]",
	Aliases:          []string{"write"},
	Short:            "do transaction to interact with smart contracts",
	Long:             ` `,
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonTxPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)

		reader := tc.Reader
		if reader == nil {
			appUI.Error("Couldn't init eth reader.")
			return
		}

		a, err := tc.Resolver.ConfigToABI(tc.To, config.ForceERC20ABI, config.CustomABI, config.Network())
		if err != nil {
			appUI.Error("Couldn't get abi for %s: %s", tc.To, err)
			return
		}

		data, err := cmdutil.PromptTxData(
			appUI,
			tc.Analyzer,
			tc.To,
			config.MethodIndex,
			tc.PrefillParams,
			tc.PrefillMode,
			a,
			nil,
			config.Network(),
		)

		jarviscommon.DebugPrintf("calling data: %x\n", data)

		if err != nil {
			appUI.Error("Couldn't pack data: %s", err)
			return
		}
		if config.GasLimit == 0 {
			config.GasLimit, err = reader.EstimateExactGas(tc.From, tc.To, 0, tc.Value, data)
			if err != nil {
				appUI.Error("Couldn't estimate gas limit: %s", err)
				return
			}
		}

		tx := jarviscommon.BuildExactTx(
			tc.TxType,
			tc.Nonce,
			tc.To,
			tc.Value,
			config.GasLimit+config.ExtraGasLimit,
			tc.GasPrice+config.ExtraGasPrice,
			tc.TipGas+config.ExtraTipGas,
			data,
			config.Network().GetChainID(),
		)
		if broadcasted, err := cmdutil.SignAndBroadcast(
			appUI, tc.FromAcc, tx,
			map[string]*abi.ABI{strings.ToLower(tc.To): a},
			reader, tc.Analyzer, a, tc.Broadcaster,
		); err != nil && !broadcasted {
			appUI.Error("Failed to proceed after signing the tx: %s. Aborted.", err)
		}
	},
}

func handleReadOneFunctionOnContract(r readerPkg.Reader, analyzer util.TxAnalyzer, a *abi.ABI, atBlock int64, to string, method *abi.Method, params []interface{}) (contractReadResult, error) {
	responseBytes, err := r.ReadContractToBytes(atBlock, "0x0000000000000000000000000000000000000000", to, a, method.Name, params...)
	if err != nil {
		appUI.Error("getting response failed: %s", err)
		return contractReadResult{}, err
	}
	if len(responseBytes) == 0 {
		appUI.Error("the function reverts. please double check your params.")
		return contractReadResult{}, fmt.Errorf("the function reverted")
	}
	ps, err := method.Outputs.UnpackValues(responseBytes)
	if err != nil {
		appUI.Error("Couldn't unpack response to go types: %s", err)
		return contractReadResult{}, err
	}

	appUI.Info("Output:")
	result := contractReadResult{}
	for i, output := range method.Outputs {
		oneOutputParamResult := analyzer.ParamAsJarvisParamResult(output.Name, output.Type, ps[i])
		oneVerboseParamResult := convertToVerboseParamResult(oneOutputParamResult)
		result = append(result, oneVerboseParamResult)

		fmt.Fprintf(appUI.Writer(), "%d. ", i+1)
		jarviscommon.PrintVerboseParamResultToWriter(appUI.Writer(), oneOutputParamResult, 0, true)
		fmt.Fprintln(appUI.Writer())
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
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		appUI.Error("Writing to json file failed: %s", err)
	}
}

type batchcontractReadResultJSON struct {
	Functions []string                 `json:"functions"`
	Results   []contractReadResultJSON `json:"results"`
}

func (b *batchcontractReadResultJSON) Write(filepath string) {
	data, _ := json.MarshalIndent(b, "", "  ")
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		appUI.Error("Writing to json file failed: %s", err)
	}
}

var readContractCmd = &cobra.Command{
	Use:              "read",
	Short:            "read smart contracts (faster than etherscan)",
	Long:             ` `,
	TraverseChildren: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return cmdutil.CommonFunctionCallPreprocess(appUI, cmd, args)
	},
	Run: func(cmd *cobra.Command, args []string) {
		tc, _ := cmdutil.TxContextFrom(cmd)

		reader := tc.Reader
		if reader == nil {
			appUI.Error("Couldn't init eth reader.")
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

			a, err := tc.Resolver.ConfigToABI(tc.To, config.ForceERC20ABI, config.CustomABI, config.Network())
			if err != nil {
				appUI.Error("Couldn't get abi for %s: %s", tc.To, err)
				return
			}

			methods := cmdutil.AllZeroParamFunctions(a)
			for i := range methods {
				method := methods[i]
				resultJSON.Functions = append(resultJSON.Functions, method.Name)
				appUI.Info("%d. %s", i+1, method.Name)

				result, err := handleReadOneFunctionOnContract(reader, tc.Analyzer, a, config.AtBlock, tc.To, &method, []interface{}{})
				if err != nil {
					resultJSON.Results = append(resultJSON.Results, contractReadResultJSON{
						Error: fmt.Sprintf("%s", err),
					})
				} else {
					resultJSON.Results = append(resultJSON.Results, contractReadResultJSON{
						Result: result,
					})
				}
				appUI.Info("---------------------------------------------------")
			}
		} else {
			resultJSON := contractReadResultJSON{
				Result: contractReadResult{},
				Error:  "",
			}

			if config.JSONOutputFile != "" {
				defer resultJSON.Write(config.JSONOutputFile)
			}

			a, err := tc.Resolver.ConfigToABI(tc.To, config.ForceERC20ABI, config.CustomABI, config.Network())
			if err != nil {
				appUI.Error("Couldn't get abi for %s: %s", tc.To, err)
				return
			}

			method, params, err := cmdutil.PromptFunctionCallData(
				appUI,
				tc.Analyzer,
				tc.To,
				config.MethodIndex,
				tc.PrefillParams,
				tc.PrefillMode,
				"read",
				a,
				nil,
				config.Network(),
			)
			if err != nil {
				appUI.Error("Couldn't get params from users: %s", err)
				resultJSON.Error = fmt.Sprintf("%s", err)
				return
			}
			result, err := handleReadOneFunctionOnContract(reader, tc.Analyzer, a, config.AtBlock, tc.To, method, params)

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
	rootCmd.AddCommand(contractCmd)
}
