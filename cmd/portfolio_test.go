package cmd

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/networks"
)

func TestSelectPortfolioChains(t *testing.T) {
	mainnet := int(networks.EthereumMainnet.GetChainID())
	bsc := int(networks.BSCMainnet.GetChainID())
	base := int(networks.BaseMainnet.GetChainID())

	t.Run("neither flag keeps every chain", func(t *testing.T) {
		got, err := selectPortfolioChains(false, "mainnet", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("chains = %v, want every chain", got)
		}
	})

	t.Run("explicit network", func(t *testing.T) {
		got, err := selectPortfolioChains(true, "ethereum", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got[mainnet]; !ok || len(got) != 1 {
			t.Fatalf("chains = %v, want {%d}", got, mainnet)
		}
	})

	t.Run("network unions with chain", func(t *testing.T) {
		got, err := selectPortfolioChains(true, "bsc", []string{"base", "56"})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := got[bsc]; !ok {
			t.Fatalf("missing bsc in %v", got)
		}
		if _, ok := got[base]; !ok {
			t.Fatalf("missing base in %v", got)
		}
		if len(got) != 2 {
			t.Fatalf("chains = %v, want bsc and base once each", got)
		}
	})

	t.Run("unknown network", func(t *testing.T) {
		if _, err := selectPortfolioChains(true, "not-a-chain", nil); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestNetworkFlagChangedOnPortfolio(t *testing.T) {
	for _, args := range [][]string{
		{"-k", "bsc", "portfolio"},
		{"portfolio", "-k", "bsc"},
		{"--network", "bsc", "portfolio"},
	} {
		parent := &cobra.Command{Use: "jarvis"}
		var network string
		parent.PersistentFlags().StringVarP(&network, "network", "k", "mainnet", "")
		var changed bool
		child := &cobra.Command{Use: "portfolio"}
		child.RunE = func(cmd *cobra.Command, _ []string) error {
			changed = cmd.Flags().Changed("network")
			got, err := selectPortfolioChains(changed, network, nil)
			if err != nil {
				return err
			}
			if _, ok := got[int(networks.BSCMainnet.GetChainID())]; !ok || len(got) != 1 {
				return errPortfolioChain(args, got)
			}
			return nil
		}
		parent.AddCommand(child)
		parent.SetArgs(args)
		parent.SetOut(&discardWriter{})
		parent.SetErr(&discardWriter{})
		if err := parent.Execute(); err != nil {
			t.Fatalf("args %v: %v", args, err)
		}
		if !changed {
			t.Fatalf("args %v: -k was not seen on the portfolio command", args)
		}
	}

	parent := &cobra.Command{Use: "jarvis"}
	var network string
	parent.PersistentFlags().StringVarP(&network, "network", "k", "mainnet", "")
	child := &cobra.Command{Use: "portfolio", RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("network") {
			t.Fatal("default -k counted as set")
		}
		got, err := selectPortfolioChains(false, network, nil)
		if err != nil {
			return err
		}
		if got != nil {
			t.Fatalf("default network narrowed the book to %v", got)
		}
		return nil
	}}
	parent.AddCommand(child)
	parent.SetArgs([]string{"portfolio"})
	parent.SetOut(&discardWriter{})
	parent.SetErr(&discardWriter{})
	if err := parent.Execute(); err != nil {
		t.Fatal(err)
	}
}

func errPortfolioChain(args []string, got map[int]struct{}) error {
	return fmt.Errorf("args %v: chains = %v", args, got)
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
