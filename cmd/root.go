// Copyright © 2018 Victor Tran
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/networks"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "jarvis",
	Short: "Assist you to read ethereum contract and do ethereum tx easily",
	Long: fmt.Sprintf(`Jarvis is a command line tool to assist ethereum dapp operators to do their
daily tasks such as reading smart contracts and doing operational transactions.

Jarvis supports you on different ends:

	1. It manages your ethereum accounts so you dont have to
	remember which address is with which kind of key like keystores,
	trezor, ledger...

	2. It manages a book of addresses so you will not forget one, easily
	look them up and will show you address information to improve tx's
	verbosity.

	3. It helps you to read smart contract and do transactions with
	intuitive command line interface.

By default, Jarvis uses the following nodes to support different chains: %s
You can also add your custom node by setting the following env vars: %s
In case you want many custom nodes, you can define it in ~/nodes.json with format describe at
https://github.com/tranvictor/jarvis#configure-custom-nodes

Jarvis also utilizes chain explorers like Etherscan, Bscscan and Tomoscan in order to look
up additional informations such as contract ABI, recommended gas price..etc. By default,
Jarvis uses default API keys. You can also specify your API keys (recommended to make
Jarvis more stable) by setting the following env vars: %s

Note: Jarvis will only check if the env vars are not empty and take the env vars blindly,
it will not check if it is a valid url or not, the error will pop up during its command
execution instead.

For more information or support, reach me at https://github.com/tranvictor.`,
		SupportedNetworkAndNodesHelpString(),
		SupportedNetworkAndNodeVariableHelpString(),
		SupportedNetworkAndBlockExplorerVariableHelpString(),
	),
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

func SupportedNetworkAndNodesHelpString() string {
	result := "\n"
	for i, n := range networks.GetSupportedNetworks() {
		result += fmt.Sprintf("  %d. For %s:\n", i+1, n.GetName())
		for nodeName, url := range n.GetDefaultNodes() {
			result += fmt.Sprintf("    %s: %s\n", nodeName, url)
		}
	}
	return result
}

func SupportedNetworkAndNodeVariableHelpString() string {
	result := "\n"
	for i, n := range networks.GetSupportedNetworks() {
		result += fmt.Sprintf("  %d. For %s: %s\n", i+1, n.GetName(), n.GetNodeVariableName())
	}
	return result
}

func SupportedNetworkAndBlockExplorerVariableHelpString() string {
	result := "\n"
	for i, n := range networks.GetSupportedNetworks() {
		result += fmt.Sprintf(
			"  %d. For %s: %s\n",
			i+1,
			n.GetBlockExplorerAPIURL(),
			n.GetBlockExplorerAPIKeyVariableName(),
		)
	}
	return result
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd.PersistentFlags().StringVarP(
		&config.NetworkString,
		"network",
		"k",
		networks.EthereumMainnet.GetName(),
		fmt.Sprintf(
			"ethereum network. Valid values: %s. If the network is not supported, fall back to mainnet.",
			networks.GetSupportedNetworkNames(),
		),
	)

	rootCmd.PersistentFlags().BoolVarP(
		&config.DegenMode,
		"degen",
		"x",
		false,
		"Set to enable degen prints such as detailed contract calls, nonces... Default false",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&config.YesToAllPrompt,
		"yes",
		"Y",
		false,
		"Automatically Yes to all Y/N prompts",
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
