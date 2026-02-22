package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

var (
	NetworkConfig string
	NetworkForce  bool
)

func PromptNetwork() networks.Network {
	name := cmdutil.PromptInputWithValidation(appUI, "Please enter the name of the network", func(name string) error {
		if name == "" {
			return fmt.Errorf("name cannot be empty")
		}
		if NetworkForce {
			return nil
		}
		if _, err := networks.GetNetwork(name); err == nil {
			return fmt.Errorf("network with name %s already exists. If you want to replace it, use flag --force", name)
		}
		return nil
	})

	alternativeNamesStr := cmdutil.PromptInput(appUI, "Please enter the alternative names of the network (comma separated, don't wrap with quotes)")
	alternativeNames := strings.Split(alternativeNamesStr, ",")
	for i, name := range alternativeNames {
		alternativeNames[i] = strings.TrimSpace(name)
	}

	chainIDStr := cmdutil.PromptInputWithValidation(appUI, "Please enter the chain ID of the network", func(chainIDstr string) error {
		if chainIDstr == "" {
			return fmt.Errorf("chain ID cannot be empty")
		}
		if _, err := strconv.Atoi(chainIDstr); err != nil {
			return fmt.Errorf("chain ID must be a number")
		}
		return nil
	})
	chainID, _ := strconv.Atoi(chainIDStr)

	nativeTokenSymbol := cmdutil.PromptInputWithValidation(appUI, "Please enter the native token symbol of the network", func(nativeTokenSymbol string) error {
		if nativeTokenSymbol == "" {
			return fmt.Errorf("native token symbol cannot be empty")
		}
		return nil
	})

	nativeTokenDecimalStr := cmdutil.PromptInputWithValidation(appUI, "Please enter the native token decimal of the network", func(nativeTokenDecimal string) error {
		if nativeTokenDecimal == "" {
			return fmt.Errorf("native token decimal cannot be empty")
		}
		if _, err := strconv.Atoi(nativeTokenDecimal); err != nil {
			return fmt.Errorf("native token decimal must be a number")
		}
		return nil
	})
	nativeTokenDecimal, _ := strconv.Atoi(nativeTokenDecimalStr)

	blockTimeStr := cmdutil.PromptInputWithValidation(appUI, "Please enter the block time of the network in seconds", func(blockTime string) error {
		if blockTime == "" {
			return fmt.Errorf("block time cannot be empty")
		}
		if _, err := strconv.Atoi(blockTime); err != nil {
			return fmt.Errorf("block time must be a number")
		}
		return nil
	})
	blockTime, _ := strconv.Atoi(blockTimeStr)

	nodeVariableName := cmdutil.PromptInputWithValidation(appUI, "Please enter the node variable name of the network", func(nodeVariableName string) error {
		if nodeVariableName == "" {
			return fmt.Errorf("node variable name cannot be empty")
		}
		if strings.ToUpper(nodeVariableName) != nodeVariableName {
			return fmt.Errorf("node variable name must be in capital letters")
		}
		return nil
	})

	defaultNodesStr := cmdutil.PromptInputWithValidation(appUI, "Please enter the default node urls of the network (comma separated, no wrapping with quotes)", func(defaultNodes string) error {
		if defaultNodes == "" {
			return fmt.Errorf("default node urls cannot be empty")
		}
		for _, node := range strings.Split(defaultNodes, ",") {
			if _, err := url.Parse(node); err != nil {
				return fmt.Errorf("default node url %s is not a valid url", node)
			}
		}
		return nil
	})
	defaultNodes := make(map[string]string)
	for _, node := range strings.Split(defaultNodesStr, ",") {
		nodeURL, _ := url.Parse(strings.TrimSpace(node))
		defaultNodes[nodeURL.Host] = strings.TrimSpace(node)
	}

	blockExplorerAPIKeyVariableName := cmdutil.PromptInputWithValidation(appUI, "Please enter the block explorer API key variable name of the network", func(v string) error {
		if v == "" {
			return fmt.Errorf("block explorer API key variable name cannot be empty")
		}
		return nil
	})

	blockExplorerAPIURL := cmdutil.PromptInputWithValidation(appUI, "Please enter the block explorer API URL of the network", func(v string) error {
		if v == "" {
			return fmt.Errorf("block explorer API URL cannot be empty")
		}
		if _, err := url.Parse(v); err != nil {
			return fmt.Errorf("block explorer API URL %s is not a valid url", v)
		}
		return nil
	})

	multiCallContractAddress := cmdutil.PromptInputWithValidation(appUI, "Please enter the multi call contract address of the network", func(v string) error {
		if v == "" {
			return fmt.Errorf("multi call contract address cannot be empty")
		}
		if !common.IsHexAddress(v) {
			return fmt.Errorf("multi call contract address %s is not a valid address", v)
		}
		return nil
	})

	networkConfig := networks.GenericEtherscanNetworkConfig{
		Name:                            name,
		AlternativeNames:                alternativeNames,
		ChainID:                         uint64(chainID),
		NativeTokenSymbol:               nativeTokenSymbol,
		NativeTokenDecimal:              uint64(nativeTokenDecimal),
		BlockTime:                       uint64(blockTime),
		NodeVariableName:                nodeVariableName,
		DefaultNodes:                    defaultNodes,
		BlockExplorerAPIKeyVariableName: blockExplorerAPIKeyVariableName,
		BlockExplorerAPIURL:             blockExplorerAPIURL,
		MultiCallContractAddress:        common.HexToAddress(multiCallContractAddress),
	}

	return networks.NewGenericEtherscanNetwork(networkConfig)
}

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
		cfg, err := cmd.Flags().GetString("config")
		if err != nil {
			appUI.Error("Error: %s", err)
			return
		}

		var newNetwork networks.Network
		cfg = strings.TrimSpace(cfg)
		if cfg != "" && strings.HasPrefix(cfg, "{") && strings.HasSuffix(cfg, "}") {
			newNetwork, err = networks.NewNetworkFromJSON([]byte(cfg))
			if err != nil {
				appUI.Error("The provided json is not valid: %s", err)
				return
			}
		} else if cfg != "" {
			jsonFile, err := os.Open(cfg)
			if err != nil {
				appUI.Error("Couldn't open the provided json file: %s", err)
				return
			}
			defer jsonFile.Close()

			jsonBytes, err := io.ReadAll(jsonFile)
			if err != nil {
				appUI.Error("Couldn't read the provided json file: %s", err)
				return
			}
			newNetwork, err = networks.NewNetworkFromJSON(jsonBytes)
			if err != nil {
				appUI.Error("The provided json is not a valid network config: %s", err)
				return
			}
		} else {
			newNetwork = PromptNetwork()
		}

		allNames := []string{newNetwork.GetName()}
		allNames = append(allNames, newNetwork.GetAlternativeNames()...)

		var willReplace bool
		for _, name := range allNames {
			_, err = networks.GetNetwork(name)
			if err == nil && !NetworkForce {
				appUI.Error("Network with name %s already exists. Abort. If you want to update the network, use flag --force.", name)
				return
			}
			if err == nil && NetworkForce {
				appUI.Warn("Network with name %s already exists. We will replace it with the new network.", name)
				willReplace = true
				continue
			}
			if err != nil {
				willReplace = true
				continue
			}
		}

		if willReplace {
			if err = networks.AddNetwork(newNetwork); err != nil {
				appUI.Error("Failed to add the new network: %s", err)
				return
			}
			appUI.Success("Network %s with chain ID %d added and saved to ~/.jarvis/networks/.", newNetwork.GetName(), newNetwork.GetChainID())
		}
	},
}

var listNetworkCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all of supported networks",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		nets := networks.GetSupportedNetworks()
		for i, n := range nets {
			nodes, err := util.GetNodes(n)
			if err != nil {
				appUI.Error("Error: %s", err)
				continue
			}
			appUI.Info("%d. Name: %s, Chain ID: %d", i+1, n.GetName(), n.GetChainID())
			appUI.Info("    RPC nodes:")
			for key, node := range nodes {
				appUI.Info("    - %s: %s", key, node)
			}
		}

		appUI.Info("\nIf you want to add more networks to the list, use following command:\n> jarvis network add")
		appUI.Info("\nIf you want to delete a network, just delete the corresponding json file in ~/.jarvis/networks/.")
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
