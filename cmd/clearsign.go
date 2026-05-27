// Copyright © 2026 Victor Tran
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

package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/txanalyzer/erc7730"
	"github.com/tranvictor/jarvis/ui"
)

var clearsignDisableContract string

// clearsignCmd groups the subcommands that operate on the local
// ERC-7730 descriptor registry. The registry is the source of truth
// for the clear-signed view shown at sign time by `jarvis send`,
// `jarvis msig` and `jarvis wc`.
var clearsignCmd = &cobra.Command{
	Use:   "clearsign",
	Short: "Manage ERC-7730 clear-signing descriptors used by jarvis",
	Long: `clearsign manages the on-disk ERC-7730 registry that powers the
green-bordered "Clear Signed" panel jarvis shows before every
transaction or typed-data signature.

Descriptors live under ~/.jarvis/erc7730/:
  registry/  — mirror of github.com/ethereum/clear-signing-erc7730-registry
  local/     — descriptors you added with 'jarvis clearsign add', or
               disabled-overrides for upstream descriptors you don't trust.

The registry is fetched lazily on first miss and after that whenever
you run 'jarvis clearsign update'. Local descriptors override the
registry on the same (chainId, address) key.`,
}

var clearsignUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Sync the ERC-7730 registry from upstream",
	Long: `Downloads the latest tarball from the public registry at
` + erc7730.RegistryArchiveURL + ` and atomically replaces the local
mirror. Existing user-added descriptors under local/ are untouched.

Network access is required.`,
	Run: func(cmd *cobra.Command, args []string) {
		reg := erc7730.SharedRegistry()
		stop := appUI.Spinner("Fetching ERC-7730 registry...")
		ctx := context.Background()
		count, err := reg.SyncRegistry(ctx, erc7730.SyncOptions{
			OnProgress: func(stage string, n int) {},
		})
		stop()
		if err != nil {
			appUI.Error("clearsign update failed: %s", err)
			os.Exit(1)
		}
		reg.TouchLastSync()
		appUI.Success("Registry updated: %d descriptors at %s",
			count, reg.RegistryDir())
	},
}

var clearsignAddCmd = &cobra.Command{
	Use:   "add <descriptor.json>",
	Short: "Add a local ERC-7730 descriptor",
	Long: `Copies <descriptor.json> into ~/.jarvis/erc7730/local/ after
validating that it parses as an ERC-7730 file. Local descriptors
override the synced registry on conflict.

Use this for unpublished contracts (testnets, work-in-progress, or
private deployments) until the descriptor is upstreamed.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		raw, err := os.ReadFile(args[0])
		if err != nil {
			appUI.Error("read %s: %s", args[0], err)
			os.Exit(1)
		}
		path, err := erc7730.SharedRegistry().AddLocalFromBytes("", raw)
		if err != nil {
			appUI.Error("invalid descriptor: %s", err)
			os.Exit(1)
		}
		appUI.Success("Installed %s", path)
	},
}

var clearsignListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed ERC-7730 descriptors",
	Long: `Lists every descriptor jarvis knows about — both the upstream
registry mirror and any descriptors you added locally — alongside
the contract(s) / EIP-712 domain(s) each one binds to.`,
	Run: func(cmd *cobra.Command, args []string) {
		all := erc7730.SharedRegistry().All()
		if len(all) == 0 {
			appUI.Warn("No descriptors installed. Run 'jarvis clearsign update' to sync the public registry.")
			return
		}
		// Sort by Owner / ContractName for predictable output.
		sort.SliceStable(all, func(i, j int) bool {
			return descriptorSortKey(all[i]) < descriptorSortKey(all[j])
		})
		rows := make([][]ui.TableCell, 0, len(all))
		for _, d := range all {
			rows = append(rows, []ui.TableCell{
				ui.TC(descriptorSortKey(d)),
				ui.TC(d.Source),
				ui.TC(bindingSummary(d)),
				ui.TC(fmt.Sprintf("%d format(s)", len(d.Display.Formats))),
			})
		}
		appUI.Table(
			[]string{"Owner / Contract", "Source", "Binding", "Formats"},
			tableCellsToStrings(rows),
		)
	},
}

var clearsignShowCmd = &cobra.Command{
	Use:   "show <chainId> <address>",
	Short: "Show the descriptor jarvis would use for an address on a chain",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		chainID, err := parseUint64(args[0])
		if err != nil {
			appUI.Error("chainId: %s", err)
			os.Exit(1)
		}
		addr := args[1]
		descriptors := erc7730.SharedRegistry().FindByContract(chainID, addr)
		if len(descriptors) == 0 {
			appUI.Warn("No descriptor found for %s on chain %d.", addr, chainID)
			return
		}
		for i, d := range descriptors {
			if i > 0 {
				appUI.Info("")
			}
			renderDescriptorSummary(d)
		}
	},
}

var clearsignDisableCmd = &cobra.Command{
	Use:   "disable <chainId> <address>",
	Short: "Stop jarvis from clear-signing transactions to a contract",
	Long: `Adds a local opt-out descriptor for (chainId, address) so the
upstream registry's entry is ignored. The contract will fall back to
the raw ABI-decoded view at signing time.

Run 'jarvis clearsign update' to re-enable later, or remove the
generated file under ~/.jarvis/erc7730/local/ by hand.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		_, err := parseUint64(args[0])
		if err != nil {
			appUI.Error("chainId: %s", err)
			os.Exit(1)
		}
		// We implement opt-out as a stub descriptor that binds to
		// the same (chainId, address) but provides no formats. The
		// matcher will still find this descriptor first (local
		// directory is loaded before registry/), and FindContractMatch
		// will skip it because no format selector matches. End
		// result: the green panel is suppressed without us having
		// to add a parallel deny-list system.
		stub := fmt.Sprintf(`{
  "context": {"$id": "user-disable",
              "contract": {"deployments": [{"chainId": %s, "address": "%s"}]}},
  "metadata": {"owner": "Disabled by user"},
  "display": {"formats": {"disabled()": {"intent": "Disabled"}}}
}`, args[0], args[1])
		path, err := erc7730.SharedRegistry().AddLocalFromBytes(
			fmt.Sprintf("disabled-%s-%s.json", args[0], strings.ToLower(strings.TrimPrefix(args[1], "0x"))),
			[]byte(stub),
		)
		if err != nil {
			appUI.Error("disable failed: %s", err)
			os.Exit(1)
		}
		appUI.Success("Disabled: %s", path)
	},
}

func renderDescriptorSummary(d *erc7730.Descriptor) {
	appUI.Critical("%s", descriptorSortKey(d))
	if d.Metadata.Info != nil && d.Metadata.Info.URL != "" {
		appUI.Info("URL    : %s", d.Metadata.Info.URL)
	}
	appUI.Info("Source : %s · cached %s", d.Source,
		time.Unix(d.CachedAtUnix, 0).Format("2006-01-02"))
	appUI.Info("Binding: %s", bindingSummary(d))
	appUI.Info("Formats:")
	for key := range d.Display.Formats {
		appUI.Info("  - %s", key)
	}
}

func descriptorSortKey(d *erc7730.Descriptor) string {
	switch {
	case d.Metadata.Owner != "" && d.Metadata.ContractName != "" &&
		!strings.EqualFold(d.Metadata.Owner, d.Metadata.ContractName):
		return d.Metadata.Owner + " · " + d.Metadata.ContractName
	case d.Metadata.Owner != "":
		return d.Metadata.Owner
	case d.Metadata.ContractName != "":
		return d.Metadata.ContractName
	case d.Context.ID != "":
		return d.Context.ID
	}
	return "(anonymous descriptor)"
}

func bindingSummary(d *erc7730.Descriptor) string {
	if d.Context.Contract != nil {
		parts := make([]string, 0, len(d.Context.Contract.Deployments))
		for _, dep := range d.Context.Contract.Deployments {
			parts = append(parts, fmt.Sprintf("chain %d / %s", dep.ChainID, shortContractAddr(dep.Address)))
		}
		return strings.Join(parts, ", ")
	}
	if d.Context.EIP712 != nil {
		if d.Context.EIP712.Domain != nil && d.Context.EIP712.Domain.Name != "" {
			return "EIP-712 domain " + d.Context.EIP712.Domain.Name
		}
		if len(d.Context.EIP712.Deployments) > 0 {
			parts := make([]string, 0, len(d.Context.EIP712.Deployments))
			for _, dep := range d.Context.EIP712.Deployments {
				parts = append(parts, fmt.Sprintf("EIP-712 chain %d / %s", dep.ChainID, shortContractAddr(dep.Address)))
			}
			return strings.Join(parts, ", ")
		}
	}
	return "(no binding)"
}

func shortContractAddr(s string) string {
	if len(s) < 12 {
		return s
	}
	return s[:6] + "…" + s[len(s)-4:]
}

func tableCellsToStrings(rows [][]ui.TableCell) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		out[i] = make([]string, len(r))
		for j, c := range r {
			out[i][j] = c.Text
		}
	}
	return out
}

func parseUint64(s string) (uint64, error) {
	var n uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("expected digits, got %q", s)
		}
		n = n*10 + uint64(c-'0')
	}
	return n, nil
}

func init() {
	_ = config.NetworkString // suppress unused import; CLI uses --network via root flags only
	clearsignCmd.AddCommand(clearsignUpdateCmd)
	clearsignCmd.AddCommand(clearsignAddCmd)
	clearsignCmd.AddCommand(clearsignListCmd)
	clearsignCmd.AddCommand(clearsignShowCmd)
	clearsignCmd.AddCommand(clearsignDisableCmd)
	clearsignDisableCmd.Flags().StringVar(&clearsignDisableContract, "name", "", "optional override for the generated descriptor filename")
	rootCmd.AddCommand(clearsignCmd)
}
