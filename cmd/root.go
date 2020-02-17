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
	"github.com/tranvictor/jarvis/util"
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

By default, Jarvis supports ethereum mainnet, ropsten and tomo chain. In order to
interact with the chains, it uses the following nodes by default:
	1. For mainnet: it uses alchemy and infura
	2. For ropsten: it uses infura
	3. For tomo: it uses rpc.tomochain.com
You can also add your custom node by setting the following env vars:
	1. For mainnet: %s,
	2. For ropsten: %s,
	3. For tomo: %s,
Note: jarvis will only check if the env vars are not empty and take the env vars blindly,
it will not check if it is a valid url or not, the error will pop up during its command
execution instead.

For more information or support, reach me at https://github.com/tranvictor.`,
		util.ETHEREUM_MAINNET_NODE_VAR,
		util.ETHEREUM_ROPSTEN_NODE_VAR,
		util.TOMO_MAINNET_NODE_VAR,
	),
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	rootCmd.PersistentFlags().StringVarP(&config.Network, "network", "k", "mainnet", "ethereum network. Valid values: \"mainnet\", \"ropsten\", \"tomo\".")

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
