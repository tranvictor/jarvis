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
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
)

// appUI is the package-level UI used by all cmd/* files. It is initialised
// once at startup as a TerminalUI and can be replaced in tests with a
// RecordingUI.
var appUI ui.UI = ui.NewTerminalUI()

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "jarvis",
	Short: "Assist you to read ethereum contract and do ethereum tx easily",
	Long: fmt.Sprintf(`Jarvis is a command line tool for people who operate Ethereum contracts:
it reads transactions and contracts, builds and signs transactions from
keystores, Ledger and Trezor, and drives Gnosis Safe / classic multisigs.

  jarvis info <tx hash>          what a transaction did, decoded
  jarvis send                    move ETH or tokens from a wallet or a Safe
  jarvis contract read|tx        call or write any verified contract
  jarvis msig                    propose, approve and execute multisig txs
  jarvis wallet / addr           the keys you sign with, the names you trust

Networks (pick one with -k/--network):
%s
"jarvis network list" shows chain IDs and RPC nodes; "jarvis node --help"
adds or tests nodes. Nodes live in ~/.jarvis/nodes/<network>.json and can be
overridden with the network's node env var (e.g. ETHEREUM_MAINNET_NODE).

Block-explorer API keys (set your own for reliable ABI lookups):
%s

For more information or support, reach me at https://github.com/tranvictor.`,
		wrapList(strings.Split(networks.SupportedNetworkNamesHelp(), ", "), "  ", 76),
		wrapList(blockExplorerKeyVariables(), "  ", 76),
	),
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// wrapList joins items with ", " into lines no wider than width, each
// prefixed with indent.
func wrapList(items []string, indent string, width int) string {
	var lines []string
	cur := ""
	for _, it := range items {
		switch {
		case cur == "":
			cur = indent + it
		case len(cur)+2+len(it) > width:
			lines = append(lines, cur+",")
			cur = indent + it
		default:
			cur += ", " + it
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

// blockExplorerKeyVariables lists each explorer API-key env var once.
func blockExplorerKeyVariables() []string {
	seen := map[string]bool{}
	names := []string{}
	for _, n := range networks.GetSupportedNetworks() {
		v := n.GetBlockExplorerAPIKeyVariableName()
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		names = append(names, v)
	}
	sort.Strings(names)
	return names
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// Hardware-wallet waits (Ledger not yet connected) report through the
	// same status spinner as the rest of the app.
	ledgereum.ProgressUI = appUI

	rootCmd.PersistentFlags().StringVarP(
		&config.NetworkString,
		"network",
		"k",
		networks.EthereumMainnet.GetName(),
		fmt.Sprintf(
			"network to operate on: %s",
			networks.SupportedNetworkNamesHelp(),
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

	rootCmd.PersistentFlags().BoolVarP(
		&config.Debug,
		"debug",
		"B",
		false,
		"print debug logs to screen, helpful to diagnose performance issues",
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
