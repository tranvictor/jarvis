package cmd

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/safe/chainregistry"
)

var chainsMsigCmd = &cobra.Command{
	Use:   "chains",
	Short: "Inspect and refresh the Safe-supported chain list jarvis uses",
	Long: `The 'chains' subcommand manages jarvis's knowledge of which EVM chains
Gnosis Safe supports and where each chain's Transaction Service lives.

jarvis ships with a built-in baseline list for offline / airgapped use.
Running 'jarvis msig chains refresh' pulls the latest list from Safe's
Config Service (https://safe-client.safe.global/v1/chains) and caches it
on disk, so newly Safe-supported chains can be recognised without a
jarvis release.`,
}

var listChainsMsigCmd = &cobra.Command{
	Use:   "list",
	Short: "List every chain the Safe registry currently knows about",
	Run: func(cmd *cobra.Command, args []string) {
		entries := chainregistry.All()
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ChainID < entries[j].ChainID
		})

		fetchedAt := chainregistry.FetchedAt()
		if fetchedAt.IsZero() {
			appUI.Info("Showing built-in baseline only (never refreshed from Safe).")
			appUI.Info("Run 'jarvis msig chains refresh' to pull the live list.")
		} else {
			appUI.Info(
				"Last refreshed: %s (%s ago)",
				fetchedAt.Local().Format(time.RFC3339),
				time.Since(fetchedAt).Round(time.Minute),
			)
			if chainregistry.CacheExpired() {
				appUI.Warn("Cache is older than the 7-day TTL; consider running 'jarvis msig chains refresh'.")
			}
		}
		appUI.Info("%d chains known:", len(entries))
		fmt.Printf("%-10s  %-10s  %s\n", "chainID", "shortName", "txService")
		for _, ci := range entries {
			svc := ci.TransactionService
			if svc == "" {
				svc = "(no tx service)"
			}
			fmt.Printf("%-10d  %-10s  %s\n", ci.ChainID, ci.ShortName, svc)
		}
	},
}

var refreshChainsMsigCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Fetch the latest Safe-supported chain list and cache it on disk",
	Long: `Fetches the latest list of Safe-supported chains from the Safe
Config Service and stores it under safe:chains:v1 in ~/.jarvis/cache.json.
Subsequent lookups (including URL resolution for jarvis msig on Safe
multisigs) will prefer the cached list over jarvis's built-in baseline.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		n, err := chainregistry.Refresh(ctx)
		if err != nil {
			appUI.Error("Couldn't refresh Safe chain registry: %s", err)
			return
		}
		appUI.Success("Refreshed %d chains from Safe Config Service.", n)
		appUI.Info("Cache updated at %s", chainregistry.FetchedAt().Local().Format(time.RFC3339))
	},
}

func init() {
	chainsMsigCmd.AddCommand(listChainsMsigCmd)
	chainsMsigCmd.AddCommand(refreshChainsMsigCmd)
	msigCmd.AddCommand(chainsMsigCmd)
}
