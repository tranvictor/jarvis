package util

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/reader"
)

// NodeConfig holds per-network RPC node configuration stored in
// ~/.jarvis/nodes/<network>.json.
type NodeConfig struct {
	// Nodes is the user's custom node set: name → URL.
	Nodes map[string]string `json:"nodes"`
	// UseDefaults controls whether the network's built-in default nodes are
	// included alongside the user's custom nodes. It defaults to true so that
	// adding a custom node doesn't silently remove the built-in fallbacks.
	UseDefaults bool `json:"use_defaults"`
}

// NodeConfigDir returns ~/.jarvis/nodes/, creating it when necessary.
func NodeConfigDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("couldn't get current user: %w", err)
	}
	dir := filepath.Join(usr.HomeDir, ".jarvis", "nodes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("couldn't create node config directory: %w", err)
	}
	return dir, nil
}

func nodeConfigPath(networkName string) (string, error) {
	dir, err := NodeConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, networkName+".json"), nil
}

// LoadNodeConfig reads the node configuration for the given network name.
// Returns an error if no config file exists yet.
func LoadNodeConfig(networkName string) (NodeConfig, error) {
	p, err := nodeConfigPath(networkName)
	if err != nil {
		return NodeConfig{}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return NodeConfig{}, err
	}
	var cfg NodeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return NodeConfig{}, fmt.Errorf("malformed node config for %s: %w", networkName, err)
	}
	if cfg.Nodes == nil {
		cfg.Nodes = map[string]string{}
	}
	return cfg, nil
}

// SaveNodeConfig writes the node configuration for the given network.
func SaveNodeConfig(networkName string, cfg NodeConfig) error {
	p, err := nodeConfigPath(networkName)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

var migrateOnce sync.Once

// bootstrapNodeConfig creates an initial node config file for the given network
// seeded from its built-in default nodes. use_defaults is set to false so the
// user owns the configuration explicitly and no longer relies on the built-in
// list at runtime. The file is written best-effort; the in-memory config is
// returned regardless of whether the write succeeded.
func bootstrapNodeConfig(network jarvisnetworks.Network) NodeConfig {
	nodes := make(map[string]string, len(network.GetDefaultNodes()))
	for k, v := range network.GetDefaultNodes() {
		nodes[k] = v
	}
	cfg := NodeConfig{Nodes: nodes, UseDefaults: false}
	// Ignore write errors — the caller can still proceed with the in-memory config.
	_ = SaveNodeConfig(network.GetName(), cfg)
	return cfg
}

// GetNodes resolves the active set of RPC nodes for a network.
//
// On the very first call for a given network (no config file present) the
// built-in default nodes are written to ~/.jarvis/nodes/<network>.json so the
// user has a concrete file to inspect and edit. From that point on, only the
// user's own config file is used (use_defaults: false by default).
//
// Priority (lowest → highest):
//  1. User's config file (~/.jarvis/nodes/<network>.json); bootstrapped from
//     built-in defaults on first use if the file does not yet exist.
//  2. Built-in DefaultNodes merged in only when use_defaults: true is set.
//  3. Env-var node (NETWORK_NODE_VAR), always applied on top.
func GetNodes(network jarvisnetworks.Network) (map[string]string, error) {
	migrateOnce.Do(MigrateFromLegacyNodesJSON)

	cfg, err := LoadNodeConfig(network.GetName())
	if err != nil {
		// No config file yet — seed one from the built-in defaults.
		cfg = bootstrapNodeConfig(network)
	}

	nodes := make(map[string]string, len(cfg.Nodes))
	for k, v := range cfg.Nodes {
		nodes[k] = v
	}
	if cfg.UseDefaults {
		for k, v := range network.GetDefaultNodes() {
			if _, exists := nodes[k]; !exists {
				nodes[k] = v
			}
		}
	}

	// Env-var override is always applied on top.
	if envNode := strings.TrimSpace(os.Getenv(network.GetNodeVariableName())); envNode != "" {
		nodes["custom-node"] = envNode
	}

	return nodes, nil
}

// TestNode dials a single RPC node and measures its round-trip latency using
// a lightweight eth_getBalance call. Returns the latency and any error.
func TestNode(name, url string, network jarvisnetworks.Network) (time.Duration, error) {
	r := reader.NewEthReaderGeneric(map[string]string{name: url}, network)
	start := time.Now()
	_, err := r.GetMinedNonce("0x000000000000000000000000000000000000dead")
	return time.Since(start), err
}

// MigrateFromLegacyNodesJSON checks for the old ~/nodes.json file and, if
// found, converts each network entry into a ~/.jarvis/nodes/<network>.json
// file. The old file is renamed to ~/nodes.json.bak so it is not migrated
// again on subsequent runs.
func MigrateFromLegacyNodesJSON() {
	usr, err := user.Current()
	if err != nil {
		return
	}
	legacyPath := filepath.Join(usr.HomeDir, "nodes.json")
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return // no legacy file — nothing to migrate
	}

	var legacy map[string]map[string]string
	if err := json.Unmarshal(data, &legacy); err != nil {
		fmt.Printf("WARNING: could not parse legacy ~/nodes.json for migration: %s\n", err)
		return
	}

	migrated := 0
	for networkName, nodeMap := range legacy {
		cfg := NodeConfig{
			Nodes:       nodeMap,
			UseDefaults: false, // legacy behaviour replaced defaults entirely
		}
		if err := SaveNodeConfig(networkName, cfg); err != nil {
			fmt.Printf("WARNING: migration failed for network %q: %s\n", networkName, err)
			continue
		}
		migrated++
	}
	if migrated > 0 {
		bakPath := legacyPath + ".bak"
		os.Rename(legacyPath, bakPath)
		fmt.Printf(
			"INFO: Migrated ~/nodes.json (%d networks) to ~/.jarvis/nodes/. Old file saved as ~/nodes.json.bak.\n",
			migrated,
		)
	}
}
