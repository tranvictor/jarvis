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

	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/broadcaster"
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
	// Tags maps a node name to its tags. A present key replaces built-in
	// tags for that name, including an empty list, which clears them.
	// The tag "mev-protected" is tried first when a transaction is broadcast.
	Tags map[string][]string `json:"tags,omitempty"`
}

// nodeConfigDirForTest, when set, replaces ~/.jarvis/nodes. Tests in this
// package use it so they never write the real home directory.
var nodeConfigDirForTest func() (string, error)

// NodeConfigDir returns ~/.jarvis/nodes/, creating it when necessary.
func NodeConfigDir() (string, error) {
	if nodeConfigDirForTest != nil {
		return nodeConfigDirForTest()
	}
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

func normalizeRPCURL(u string) string {
	return strings.TrimRight(jarviscommon.CanonicalRPCURL(strings.TrimSpace(u)), "/")
}

// NormalizeTag trims a tag and canonicalizes the mev-protected tag.
// The boolean is false when tag is empty.
func NormalizeTag(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", false
	}
	if strings.EqualFold(tag, broadcaster.TagMEVProtected) {
		return broadcaster.TagMEVProtected, true
	}
	return tag, true
}

// NormalizeTags trims tags, canonicalizes mev-protected, and drops
// duplicates. Comparison is case-insensitive.
func NormalizeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, tag := range tags {
		norm, ok := NormalizeTag(tag)
		if !ok {
			continue
		}
		key := strings.ToLower(norm)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, norm)
	}
	return out
}

// UnionTags returns the case-insensitive union of a and b.
func UnionTags(a, b []string) []string {
	return NormalizeTags(append(append([]string{}, a...), b...))
}

// SubtractTags returns have without any tag listed in remove.
// The result is an empty non-nil slice when nothing remains, so callers
// can store it and suppress a built-in tag.
func SubtractTags(have, remove []string) []string {
	drop := map[string]bool{}
	for _, tag := range remove {
		norm, ok := NormalizeTag(tag)
		if !ok {
			continue
		}
		drop[strings.ToLower(norm)] = true
	}
	var kept []string
	for _, tag := range NormalizeTags(have) {
		if !drop[strings.ToLower(tag)] {
			kept = append(kept, tag)
		}
	}
	if kept == nil {
		return []string{}
	}
	return kept
}

// SameTags reports whether a and b carry the same tags, ignoring case,
// surrounding space, and order.
func SameTags(a, b []string) bool {
	left := map[string]bool{}
	for _, tag := range NormalizeTags(a) {
		left[strings.ToLower(tag)] = true
	}
	right := NormalizeTags(b)
	if len(left) != len(right) {
		return false
	}
	for _, tag := range right {
		if !left[strings.ToLower(tag)] {
			return false
		}
	}
	return true
}

func cloneTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

// EffectiveTags returns the tags for one resolved node.
// A key in userTags replaces built-in tags for that name, and an empty
// list clears them. Otherwise a built-in tag applies when url still
// matches the built-in node of the same name.
func EffectiveTags(name, url string, userTags, defaultTags map[string][]string, defaultNodes map[string]string) []string {
	if userTags != nil {
		if tags, ok := userTags[name]; ok {
			return cloneTags(tags)
		}
	}
	if defaultNodes == nil {
		return nil
	}
	defURL, ok := defaultNodes[name]
	if !ok || normalizeRPCURL(defURL) != normalizeRPCURL(url) {
		return nil
	}
	return cloneTags(defaultTags[name])
}

// EffectiveTagMap resolves tags for the active node set. Names with no
// tags are omitted.
func EffectiveTagMap(network jarvisnetworks.Network, nodes map[string]string) map[string][]string {
	var userTags map[string][]string
	if cfg, err := LoadNodeConfig(network.GetName()); err == nil {
		userTags = cfg.Tags
	}
	var defaultTags map[string][]string
	var defaultNodes map[string]string
	if network != nil {
		defaultTags = network.GetDefaultNodeTags()
		defaultNodes = network.GetDefaultNodes()
	}
	out := map[string][]string{}
	for name, url := range nodes {
		tags := EffectiveTags(name, url, userTags, defaultTags, defaultNodes)
		if len(tags) == 0 {
			continue
		}
		out[name] = tags
	}
	return out
}

// BuiltinNodeTags copies built-in tags for the nodes in the map.
// Used when seeding a node config from the built-in defaults.
func BuiltinNodeTags(network jarvisnetworks.Network, nodes map[string]string) map[string][]string {
	if network == nil || len(nodes) == 0 {
		return nil
	}
	defaults := network.GetDefaultNodeTags()
	var tags map[string][]string
	for name := range nodes {
		t := defaults[name]
		if len(t) == 0 {
			continue
		}
		if tags == nil {
			tags = map[string][]string{}
		}
		tags[name] = append([]string{}, t...)
	}
	return tags
}

// FindNodeNameByURL returns the configured name whose RPC URL matches url
// (trailing slashes, surrounding space and a missing http(s) scheme ignored).
func FindNodeNameByURL(nodes map[string]string, url string) (string, bool) {
	want := normalizeRPCURL(url)
	if want == "" {
		return "", false
	}
	for name, u := range nodes {
		if normalizeRPCURL(u) == want {
			return name, true
		}
	}
	return "", false
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
	cfg := NodeConfig{Nodes: nodes, UseDefaults: false, Tags: BuiltinNodeTags(network, nodes)}
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
		nodes[k] = jarviscommon.CanonicalRPCURL(v)
	}
	if cfg.UseDefaults {
		for k, v := range network.GetDefaultNodes() {
			if _, exists := nodes[k]; !exists {
				nodes[k] = jarviscommon.CanonicalRPCURL(v)
			}
		}
	}

	// Env-var override is always applied on top.
	if envNode := strings.TrimSpace(os.Getenv(network.GetNodeVariableName())); envNode != "" {
		nodes["custom-node"] = jarviscommon.CanonicalRPCURL(envNode)
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
