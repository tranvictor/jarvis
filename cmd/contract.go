package cmd

import (
	"fmt"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
	"github.com/tranvictor/ethutils/txanalyzer"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/util"
)

type Methods []abi.Method

func (self Methods) Len() int           { return len(self) }
func (self Methods) Swap(i, j int)      { self[i], self[j] = self[j], self[i] }
func (self Methods) Less(i, j int) bool { return self[i].Name < self[j].Name }

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
		Not supported yet

	7. fixed length bytes
		Not supported yet
	`,
	Run: func(cmd *cobra.Command, args []string) {
		analyzer := txanalyzer.NewAnalyzer()
		contractAddress, err := getContractFromParams(args)
		if err != nil {
			return
		}
		a, err := util.GetABI(contractAddress, Network)
		if err != nil {
			fmt.Printf("Couldn't get the ABI: %s\n", err)
			return
		}
		fmt.Printf("write functions:\n")
		methods := []abi.Method{}
		for _, m := range a.Methods {
			if !m.Const {
				methods = append(methods, m)
			}
		}
		sort.Sort(Methods(methods))
		for i, m := range methods {
			fmt.Printf("%d. %s\n", i+1, m.Name)
		}
		si := promptIndex(fmt.Sprintf("Please choose method index [%d, %d]", 1, len(methods)), 1, len(methods))
		method := methods[si-1]
		inputs := method.Inputs
		params := []interface{}{}
		pi := 0
		for {
			if pi >= len(inputs) {
				break
			}
			input := inputs[pi]
			fmt.Printf("%d. %s (%s)", pi, input.Name, input.Type.String())
			inputParam, err := promptParam(input)
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
		data, err := a.Pack(method.Name, params...)
		if err != nil {
			fmt.Printf("Couldn't pack data: %s\n", err)
			return
		}
		fmt.Printf("Data to sign: %s\n", common.Bytes2Hex(data))
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

func init() {
	contractCmd.AddCommand(composeDataContractCmd)
	// contractCmd.AddCommand(transactionInfocontractCmd)
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
