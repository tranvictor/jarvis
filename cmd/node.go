package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

var nodeOutputFile string
var nodeListIncludeDefaults bool

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Manage RPC nodes for each network",
	Long: `Manage the RPC nodes Jarvis uses to connect to each blockchain network.

Node configurations are stored in ~/.jarvis/nodes/<network>.json.
Each network config has a list of custom nodes and a use_defaults flag that
controls whether the built-in fallback nodes are used alongside your custom ones.

Sharing configs: use "jarvis node export" to produce a JSON file you can send to
others, and "jarvis node import <file>" to apply a shared config (nodes are merged,
never replaced).`,
}

// ── list ─────────────────────────────────────────────────────────────────────

var nodeListCmd = &cobra.Command{
	Use:   "list [network [node-name]]",
	Short: "List all configured nodes for a network (or all networks with custom configs)",
	Long: `List RPC nodes for one or all networks.

With no arguments: show a table of all networks that have custom configs.
With <network>: show a table of all active nodes for that network.
With <network> <node-name>: show a detailed single-node view (no truncation,
  connectivity is always probed).

Add --include-defaults to also show built-in default nodes and probe each one.`,
	Run: func(cmd *cobra.Command, args []string) {
		// ── single-node detail view ───────────────────────────────────────────
		if len(args) == 2 {
			networkName, nodeName := args[0], args[1]
			network, err := networks.GetNetwork(networkName)
			if err != nil {
				appUI.Error("Network %q not found: %s", networkName, err)
				return
			}
			allNodes, _ := util.GetNodes(network)
			nodeURL, ok := allNodes[nodeName]
			if !ok {
				appUI.Error("Node %q not found for network %q.", nodeName, networkName)
				appUI.Info("  Run 'jarvis node list %s' to see all configured nodes.", networkName)
				return
			}
			nodeType := resolveNodeType(network, nodeName)
			row := nodeRow{
				networkName: networkName,
				net:         network,
				name:        nodeName,
				url:         nodeURL,
				nodeType:    nodeType,
			}
			// Always probe in detail view — one node is fast.
			rows := probeNodeRows([]nodeRow{row})
			printNodeDetail(rows[0])
			return
		}

		// ── table view ───────────────────────────────────────────────────────
		if len(args) == 0 {
			if nodeListIncludeDefaults {
				var rows []nodeRow
				for _, n := range networks.GetSupportedNetworks() {
					rows = append(rows, collectNetworkNodes(n, true)...)
				}
				rows = probeNodeRows(rows)
				appUI.PrintTable(buildNodeTable(rows, true))
				return
			}
			dir, err := util.NodeConfigDir()
			if err != nil {
				appUI.Error("Couldn't access node config directory: %s", err)
				return
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) == 0 {
				appUI.Warn("No custom node configurations found.")
				appUI.Info("  Add a node:    jarvis node add <network> <name> <url>")
				appUI.Info("  Show defaults: jarvis node list --include-defaults")
				return
			}
			var rows []nodeRow
			for _, e := range entries {
				networkName := strings.TrimSuffix(e.Name(), ".json")
				network, err := networks.GetNetwork(networkName)
				if err != nil {
					cfg, cfgErr := util.LoadNodeConfig(networkName)
					if cfgErr == nil {
						for name, url := range cfg.Nodes {
							rows = append(rows, nodeRow{networkName: networkName, name: name, url: url, nodeType: "custom"})
						}
					}
					continue
				}
				rows = append(rows, collectNetworkNodes(network, false)...)
			}
			appUI.PrintTable(buildNodeTable(rows, false))
			return
		}

		// len(args) == 1
		network, err := networks.GetNetwork(args[0])
		if err != nil {
			appUI.Error("Network %q not found: %s", args[0], err)
			return
		}
		rows := collectNetworkNodes(network, nodeListIncludeDefaults)
		if nodeListIncludeDefaults {
			rows = probeNodeRows(rows)
		}
		appUI.PrintTable(buildNodeTable(rows, nodeListIncludeDefaults))
	},
}

// resolveNodeType determines the display type label for a node by name.
func resolveNodeType(network networks.Network, nodeName string) string {
	if nodeName == "custom-node" {
		return "env-override"
	}
	cfg, err := util.LoadNodeConfig(network.GetName())
	if err == nil {
		if _, exists := cfg.Nodes[nodeName]; exists {
			return "custom"
		}
	}
	if _, exists := network.GetDefaultNodes()[nodeName]; exists {
		return "default"
	}
	return "custom"
}

// printNodeDetail renders a single nodeRow as a key-value block with no
// truncation so every field is fully visible.
func printNodeDetail(r nodeRow) {
	appUI.Section(fmt.Sprintf("%s / %s", r.networkName, r.name))
	appUI.Info("Network:   %s", r.networkName)
	appUI.Info("Node Name: %s", r.name)
	appUI.Info("RPC URL:   %s", r.url)
	appUI.Info("Type:      %s", r.nodeType)
	if r.probed {
		if r.probeErr != nil {
			appUI.Error("Status:    ✗ %s", r.probeErr)
		} else {
			appUI.Success("Status:    ✓ %dms", r.latencyMs)
		}
	} else {
		appUI.Info("Status:    not tested")
	}
}

// nodeRow is a single table row describing one RPC node.
type nodeRow struct {
	networkName string
	displayName string           // networkName + alternative names, e.g. "ethereum (eth, mainnet)"
	net         networks.Network // nil for unrecognized networks
	name        string
	url         string
	nodeType    string // "default", "custom", "env-override"
	probed      bool
	latencyMs   int64
	probeErr    error
}

// networkDisplayName builds a display string that includes alternative names,
// e.g. "ethereum (eth, mainnet)".
func networkDisplayName(network networks.Network) string {
	alts := network.GetAlternativeNames()
	if len(alts) == 0 {
		return network.GetName()
	}
	return fmt.Sprintf("%s (%s)", network.GetName(), strings.Join(alts, ", "))
}

// collectNetworkNodes returns one nodeRow per active node for the given network.
// includeDefaults controls whether built-in default nodes are included.
func collectNetworkNodes(network networks.Network, includeDefaults bool) []nodeRow {
	cfg, cfgErr := util.LoadNodeConfig(network.GetName())
	useDefaults := cfgErr != nil || cfg.UseDefaults

	var rows []nodeRow

	display := networkDisplayName(network)

	if includeDefaults && useDefaults {
		for name, url := range network.GetDefaultNodes() {
			rows = append(rows, nodeRow{
				networkName: network.GetName(),
				displayName: display,
				net:         network,
				name:        name,
				url:         url,
				nodeType:    "default",
			})
		}
	}

	if cfgErr == nil {
		for name, url := range cfg.Nodes {
			rows = append(rows, nodeRow{
				networkName: network.GetName(),
				displayName: display,
				net:         network,
				name:        name,
				url:         url,
				nodeType:    "custom",
			})
		}
	}

	if envNode := strings.TrimSpace(os.Getenv(network.GetNodeVariableName())); envNode != "" {
		rows = append(rows, nodeRow{
			networkName: network.GetName(),
			displayName: display,
			net:         network,
			name:        "custom-node",
			url:         envNode,
			nodeType:    "env-override",
		})
	}

	return rows
}

// probeNodeRows tests every row concurrently and returns the updated slice.
func probeNodeRows(rows []nodeRow) []nodeRow {
	type probe struct {
		idx       int
		latencyMs int64
		err       error
	}
	ch := make(chan probe, len(rows))
	var wg sync.WaitGroup
	for i, r := range rows {
		if r.net == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, row nodeRow) {
			defer wg.Done()
			elapsed, err := util.TestNode(row.name, row.url, row.net)
			ch <- probe{idx, elapsed.Milliseconds(), err}
		}(i, r)
	}
	wg.Wait()
	close(ch)
	for p := range ch {
		rows[p.idx].probed = true
		rows[p.idx].latencyMs = p.latencyMs
		rows[p.idx].probeErr = p.err
	}
	return rows
}

// buildNodeTable assembles a ui.Table from the collected node rows.
// The Network column uses displayName (includes alt names) when available,
// falling back to networkName for unrecognized networks.
// NOTE: the table's group-separator logic keys on the first-column text, so
// all rows of a network must share the same display string — which they do
// because collectNetworkNodes sets displayName uniformly per network.
func buildNodeTable(rows []nodeRow, showStatus bool) *ui.Table {
	t := &ui.Table{
		Headers: []string{"Network", "Node Name", "RPC URL", "Type", "Status"},
	}
	for _, r := range rows {
		display := r.displayName
		if display == "" {
			display = r.networkName
		}

		var statusCell ui.TableCell
		switch {
		case !showStatus || !r.probed:
			statusCell = ui.TC("—")
		case r.probeErr != nil:
			statusCell = ui.TCS("✗ "+r.probeErr.Error(), ui.SeverityError)
		default:
			statusCell = ui.TCS(fmt.Sprintf("✓ %dms", r.latencyMs), ui.SeveritySuccess)
		}
		t.AddRow(
			ui.TC(display),
			ui.TC(r.name),
			ui.TC(r.url),
			ui.TC(r.nodeType),
			statusCell,
		)
	}
	return t
}

// ── add ──────────────────────────────────────────────────────────────────────

var nodeAddCmd = &cobra.Command{
	Use:   "add <network> <name> <url>",
	Short: "Add a custom RPC node for a network",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		networkName, name, nodeURL := args[0], args[1], args[2]
		if _, err := networks.GetNetwork(networkName); err != nil {
			appUI.Warn("Network %q is not in the built-in list, but adding the node anyway.", networkName)
		}
		cfg, err := util.LoadNodeConfig(networkName)
		if err != nil {
			cfg = util.NodeConfig{Nodes: map[string]string{}, UseDefaults: true}
		}
		cfg.Nodes[name] = nodeURL
		if err := util.SaveNodeConfig(networkName, cfg); err != nil {
			appUI.Error("Couldn't save node config: %s", err)
			return
		}
		appUI.Success("Added node %q (%s) for network %q.", name, nodeURL, networkName)
		if cfg.UseDefaults {
			appUI.Info("Built-in default nodes are still included. To disable: jarvis node defaults %s off", networkName)
		}
	},
}

// ── remove ───────────────────────────────────────────────────────────────────

var nodeRemoveCmd = &cobra.Command{
	Use:   "remove <network> <name>",
	Short: "Remove a custom RPC node for a network",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		networkName, name := args[0], args[1]
		cfg, err := util.LoadNodeConfig(networkName)
		if err != nil {
			appUI.Error("No custom node configuration found for %q.", networkName)
			return
		}
		if _, exists := cfg.Nodes[name]; !exists {
			appUI.Error("Node %q not found in configuration for %q.", name, networkName)
			return
		}
		delete(cfg.Nodes, name)
		if err := util.SaveNodeConfig(networkName, cfg); err != nil {
			appUI.Error("Couldn't save node config: %s", err)
			return
		}
		appUI.Success("Removed node %q from network %q.", name, networkName)
	},
}

// ── test ─────────────────────────────────────────────────────────────────────

var nodeTestCmd = &cobra.Command{
	Use:   "test <network> [name]",
	Short: "Test RPC node connectivity and show latency",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		networkName := args[0]
		network, err := networks.GetNetwork(networkName)
		if err != nil {
			appUI.Error("Network %q not found: %s", networkName, err)
			return
		}
		allNodes, err := util.GetNodes(network)
		if err != nil {
			appUI.Error("Couldn't resolve nodes: %s", err)
			return
		}

		if len(args) == 2 {
			targetName := args[1]
			nodeURL, ok := allNodes[targetName]
			if !ok {
				appUI.Error("Node %q not found for network %q.", targetName, networkName)
				return
			}
			allNodes = map[string]string{targetName: nodeURL}
		}

		appUI.Info("Testing %d node(s) for %s...", len(allNodes), network.GetName())
		var rows []nodeRow
		for name, nodeURL := range allNodes {
			rows = append(rows, nodeRow{
				networkName: network.GetName(),
				net:         network,
				name:        name,
				url:         nodeURL,
				nodeType:    "active",
			})
		}
		rows = probeNodeRows(rows)
		appUI.PrintTable(buildNodeTable(rows, true))
	},
}

// ── export ───────────────────────────────────────────────────────────────────

var nodeExportCmd = &cobra.Command{
	Use:   "export [network]",
	Short: "Export node configuration as shareable JSON",
	Long:  "Outputs the custom node config for a network (or all networks) to stdout or a file (-o flag).",
	Run: func(cmd *cobra.Command, args []string) {
		type exportEntry struct {
			Network     string            `json:"network"`
			Nodes       map[string]string `json:"nodes"`
			UseDefaults bool              `json:"use_defaults"`
		}

		var entries []exportEntry

		if len(args) == 0 {
			dir, _ := util.NodeConfigDir()
			files, _ := os.ReadDir(dir)
			for _, f := range files {
				networkName := strings.TrimSuffix(f.Name(), ".json")
				cfg, err := util.LoadNodeConfig(networkName)
				if err == nil {
					entries = append(entries, exportEntry{
						Network:     networkName,
						Nodes:       cfg.Nodes,
						UseDefaults: cfg.UseDefaults,
					})
				}
			}
		} else {
			networkName := args[0]
			cfg, err := util.LoadNodeConfig(networkName)
			if err != nil {
				appUI.Error("No custom node configuration found for %q: %s", networkName, err)
				return
			}
			entries = append(entries, exportEntry{
				Network:     networkName,
				Nodes:       cfg.Nodes,
				UseDefaults: cfg.UseDefaults,
			})
		}

		if len(entries) == 0 {
			appUI.Warn("No custom node configurations to export.")
			return
		}

		var out interface{}
		if len(entries) == 1 {
			out = entries[0]
		} else {
			out = entries
		}

		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			appUI.Error("Couldn't serialize config: %s", err)
			return
		}

		if nodeOutputFile != "" {
			if err := os.WriteFile(nodeOutputFile, data, 0644); err != nil {
				appUI.Error("Couldn't write to file: %s", err)
				return
			}
			appUI.Success("Config exported to %s", nodeOutputFile)
		} else {
			fmt.Println(string(data))
		}
	},
}

// ── import ───────────────────────────────────────────────────────────────────

var nodeImportCmd = &cobra.Command{
	Use:   "import <file-or-json>",
	Short: "Import a shared node configuration (merges, never replaces existing nodes)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		input := args[0]

		var data []byte
		trimmed := strings.TrimSpace(input)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			data = []byte(input)
		} else {
			var err error
			data, err = os.ReadFile(input)
			if err != nil {
				appUI.Error("Couldn't read file %q: %s", input, err)
				return
			}
		}

		type importEntry struct {
			Network     string            `json:"network"`
			Nodes       map[string]string `json:"nodes"`
			UseDefaults *bool             `json:"use_defaults"`
		}

		importOne := func(e importEntry) {
			if e.Network == "" {
				appUI.Error("Import entry is missing a \"network\" field. Skipping.")
				return
			}
			cfg, err := util.LoadNodeConfig(e.Network)
			if err != nil {
				cfg = util.NodeConfig{Nodes: map[string]string{}, UseDefaults: true}
			}
			added := 0
			for name, nodeURL := range e.Nodes {
				if _, exists := cfg.Nodes[name]; !exists {
					cfg.Nodes[name] = nodeURL
					added++
				} else {
					appUI.Warn("  Node %q already exists for %q — skipping.", name, e.Network)
				}
			}
			if e.UseDefaults != nil {
				cfg.UseDefaults = *e.UseDefaults
			}
			if err := util.SaveNodeConfig(e.Network, cfg); err != nil {
				appUI.Error("Couldn't save config for %q: %s", e.Network, err)
				return
			}
			appUI.Success("Imported %d node(s) for network %q.", added, e.Network)
		}

		// Try single-entry format first.
		var single importEntry
		if err := json.Unmarshal(data, &single); err == nil && single.Network != "" {
			importOne(single)
			return
		}

		// Try array format.
		var many []importEntry
		if err := json.Unmarshal(data, &many); err != nil {
			appUI.Error("Couldn't parse import data as JSON: %s", err)
			return
		}
		for _, e := range many {
			importOne(e)
		}
	},
}

// ── bootstrap ────────────────────────────────────────────────────────────────

var nodeBootstrapOverwrite bool

var nodeBootstrapCmd = &cobra.Command{
	Use:   "bootstrap [network]",
	Short: "Create node config files from built-in defaults for all (or one) network(s)",
	Long: `Writes ~/.jarvis/nodes/<network>.json seeded from the built-in default nodes
for every supported network (or just the one specified). use_defaults is set
to false so your files are fully self-contained.

By default, existing config files are left untouched. Pass --overwrite to
replace them with the current built-in defaults.`,
	Run: func(cmd *cobra.Command, args []string) {
		var targets []networks.Network
		if len(args) == 1 {
			n, err := networks.GetNetwork(args[0])
			if err != nil {
				appUI.Error("Network %q not found: %s", args[0], err)
				return
			}
			targets = []networks.Network{n}
		} else {
			targets = networks.GetSupportedNetworks()
		}

		created, skipped, failed := 0, 0, 0
		for _, n := range targets {
			_, cfgErr := util.LoadNodeConfig(n.GetName())
			if cfgErr == nil && !nodeBootstrapOverwrite {
				appUI.Info("  %-30s already exists — skipped (use --overwrite to replace)", n.GetName())
				skipped++
				continue
			}

			nodes := make(map[string]string, len(n.GetDefaultNodes()))
			for k, v := range n.GetDefaultNodes() {
				nodes[k] = v
			}
			cfg := util.NodeConfig{Nodes: nodes, UseDefaults: false}
			if err := util.SaveNodeConfig(n.GetName(), cfg); err != nil {
				appUI.Error("  %-30s failed: %s", n.GetName(), err)
				failed++
				continue
			}
			appUI.Success("  %-30s bootstrapped (%d node(s))", n.GetName(), len(nodes))
			created++
		}

		appUI.Info("")
		appUI.Info("Done: %d bootstrapped, %d skipped, %d failed.", created, skipped, failed)
		if skipped > 0 {
			appUI.Info("Run with --overwrite to replace existing configs.")
		}
	},
}

// ── defaults ─────────────────────────────────────────────────────────────────

var nodeDefaultsCmd = &cobra.Command{
	Use:   "defaults <network> <on|off>",
	Short: "Toggle whether built-in default nodes are used alongside custom nodes",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		networkName, toggle := args[0], args[1]
		switch toggle {
		case "on", "off":
		default:
			appUI.Error("Second argument must be \"on\" or \"off\".")
			return
		}
		cfg, err := util.LoadNodeConfig(networkName)
		if err != nil {
			cfg = util.NodeConfig{Nodes: map[string]string{}, UseDefaults: true}
		}
		cfg.UseDefaults = (toggle == "on")
		if err := util.SaveNodeConfig(networkName, cfg); err != nil {
			appUI.Error("Couldn't save config: %s", err)
			return
		}
		if cfg.UseDefaults {
			appUI.Success("Built-in default nodes are now enabled for %q.", networkName)
		} else {
			appUI.Warn("Built-in default nodes are now disabled for %q. Only your custom nodes will be used.", networkName)
		}
	},
}

func init() {
	nodeListCmd.Flags().BoolVar(&nodeListIncludeDefaults, "include-defaults", false, "Show built-in default nodes alongside custom ones, and probe each node for connectivity")
	nodeExportCmd.Flags().StringVarP(&nodeOutputFile, "output", "o", "", "Write output to this file instead of stdout")
	nodeBootstrapCmd.Flags().BoolVar(&nodeBootstrapOverwrite, "overwrite", false, "Replace existing config files with the current built-in defaults")
	nodeCmd.AddCommand(nodeListCmd, nodeAddCmd, nodeRemoveCmd, nodeTestCmd, nodeExportCmd, nodeImportCmd, nodeDefaultsCmd, nodeBootstrapCmd)
	rootCmd.AddCommand(nodeCmd)
}
