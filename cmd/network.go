package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

var (
	NetworkConfig string
	NetworkForce  bool
)

var addNetworkCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a new network to the supported networks list locally",
	Long: `--config flag is supported to pass a new network config json filepath OR pass a json string. The json should be in the following format:
	{
		"name": "network_name",
		"alternative_names": ["alternative_name_1", "alternative_name_2"],
		"chain_id": 1,
		"native_token_symbol": "ETH",
		"native_token_decimal": 18,
		"block_time": 12,
		"node_variable_name": "JARVIS_NODE_1",
		"default_nodes": {
			"node_name_1": "node_url_1",
			"node_name_2": "node_url_2"
		},
		"block_explorer_api_key_variable_name": "JARVIS_ETHERSCAN_API_KEY",
		"block_explorer_api_url": "https://api.etherscan.io/api",
		"multi_call_contract_address": "0x5394753688800000000000000000000000000000000000000000000000000000"
	}`,
	Run: func(cmd *cobra.Command, args []string) {
		// check if the network config json is passed via --config flag
		config, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Printf("Error: %s\n", err)
			return
		}

		var newNetwork networks.Network
		config = strings.TrimSpace(config)
		if config != "" && strings.HasPrefix(config, "{") && strings.HasSuffix(config, "}") {
			newNetwork, err = networks.NewNetworkFromJSON([]byte(config))
			if err != nil {
				fmt.Printf("The provided json is not valid: %s\n", err)
				return
			}
		} else if config != "" {
			// in this case, config is supposed to be a path to a json file
			jsonFile, err := os.Open(config)
			if err != nil {
				fmt.Printf("Couldn't open the provided json file: %s\n", err)
				return
			}
			defer jsonFile.Close()

			jsonBytes, err := io.ReadAll(jsonFile)
			if err != nil {
				fmt.Printf("Couldn't read the provided json file: %s\n", err)
				return
			}
			newNetwork, err = networks.NewNetworkFromJSON(jsonBytes)
			if err != nil {
				fmt.Printf("The provided json is not a valid network config: %s\n", err)
				return
			}
		} else {
			// in this case, user didn't provide any config, we need to prompt user to input the network config
		}

		allNames := []string{newNetwork.GetName()}
		allNames = append(allNames, newNetwork.GetAlternativeNames()...)

		var willReplace bool
		for _, name := range allNames {
			_, err = networks.GetNetwork(name)
			if err == nil && !NetworkForce {
				fmt.Printf("Network with name %s already exists. Abort. If you want to update the network, use flag --force.\n", name)
				return
			}

			if err == nil && NetworkForce {
				fmt.Printf("Network with name %s already exists. We will replace it with the new network.\n", name)
				willReplace = true
				continue
			}

			// err is not nil means the network is not found, hence we can add it
			if err != nil {
				willReplace = true
				continue
			}
		}

		if willReplace {
			err = networks.AddNetwork(newNetwork)
			if err != nil {
				fmt.Printf("Failed to add the new network: %s\n", err)
				return
			}
			fmt.Printf("Network %s with chain ID %d added and saved to ~/.jarvis/networks/.\n", newNetwork.GetName(), newNetwork.GetChainID())
		}
	},
}

var listNetworkCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all of supported networks",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		networks := networks.GetSupportedNetworks()
		for i, n := range networks {
			nodes, err := util.GetNodes(n)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				continue
			}
			fmt.Printf("%d. Name: %s, Chain ID: %d\n", i+1, n.GetName(), n.GetChainID())
			fmt.Printf("    RPC nodes:\n")
			for key, node := range nodes {
				fmt.Printf("    - %s: %s\n", key, node)
			}
		}

		fmt.Printf("\nJarvis: If you want to add more networks to the list, use following command:\n> jarvis network add\n")
		fmt.Printf("\nJarvis: If you want to delete a network, just delete the corresponding json file in ~/.jarvis/networks/.\n")
	},
}

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage all networks that jarvis supports",
	Long:  ``,
}

func init() {
	addNetworkCmd.PersistentFlags().StringVarP(&NetworkConfig, "config", "c", "", "Path to the network config json file")
	addNetworkCmd.PersistentFlags().BoolVarP(&NetworkForce, "force", "f", false, "Force adding the network even if it already exists")

	networkCmd.AddCommand(listNetworkCmd)
	networkCmd.AddCommand(addNetworkCmd)
	rootCmd.AddCommand(networkCmd)
}
