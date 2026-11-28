package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/holdings"
	"github.com/tranvictor/jarvis/kyber"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/cache"
)

var (
	portfolioChains  []string
	portfolioMinUSD  float64
	portfolioExitMin float64
	portfolioWatch   bool
	portfolioJSON    string
)

var portfolioCmd = &cobra.Command{
	Use:   "portfolio <address | name | ens>",
	Short: "Show a wallet's portfolio from Kyber, with live prices",
	Long: `Show one wallet's portfolio using the Kyber Data API and the RPC
nodes jarvis already has.

Prices, security and exit depth come from Kyber. Amounts are read from
the chain when a node is configured for that chain. Spam and honeypots
are removed by Kyber before the book is built.

Set JARVIS_KYBER_KEY. Create a key at https://apigw-admin.kyberswap.com/
JARVIS_KYBER_URL overrides the API base (default https://api.kyberswap.com/data/v1).

  jarvis portfolio vitalik.eth
  jarvis portfolio 0xd8da…6045 -k base --min-usd 10
  jarvis portfolio me -k mainnet --chain base
  jarvis portfolio me --watch
  jarvis portfolio me -x

-k/--network limits the book to that network. --chain does the same and
can be repeated. Pass both to include each of them. With neither flag
the book covers every chain. --watch follows live prices on one screen
and rewrites the lines that changed. A green ▲ is up and a red ▼ is
down, on the 24h column and on the value and price against 24 hours
ago. The price column is the mark's price to three digits, lined up on
the decimal point. Under 0.001 the leading zeros are a subscript count.
A short terminal keeps the top of the book and the status line.
A pipe or a terminal that cannot move the cursor prints the whole book
instead.
-x/--degen prints the liquidity payload behind each exit cell: tier,
score and venueCount. A row that never calls the pair endpoint says
ignored. A call with no tier says not quoted.
--debug appends each Kyber request URL, response body, and socket frame
to ~/.jarvis/debug.out. The screen is unchanged when --debug is off.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if err := runPortfolio(cmd, args[0]); err != nil {
			appUI.Error("%s", err)
			_ = cache.Flush()
			os.Exit(1)
		}
	},
}

func runPortfolio(cmd *cobra.Command, input string) error {
	if portfolioWatch && portfolioJSON != "" {
		return fmt.Errorf("--watch writes a live screen; leave out --json-output for a snapshot file")
	}
	chains, err := selectPortfolioChains(cmd.Flags().Changed("network"), config.NetworkString, portfolioChains)
	if err != nil {
		return err
	}
	addr, name, err := util.GetAddressFromString(input)
	if err != nil {
		return err
	}
	client, err := kyber.NewClientFromEnv()
	if err != nil {
		return err
	}
	if config.Debug {
		path, err := kyber.HomeDebugPath()
		if err != nil {
			return err
		}
		if err := kyber.EnableDebug(path); err != nil {
			return err
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	spin := appUI.Spinner("Reading portfolio…")
	loader := holdings.NewLoader(client, portfolioMinUSD, portfolioExitMin, chains)
	book, err := loader.Load(ctx, addr)
	spin.Stop(ui.StyledText{})
	if err != nil {
		return err
	}
	book.Name = name
	book.Degen = config.DegenMode
	if portfolioJSON != "" {
		if err := holdings.WriteJSON(portfolioJSON, book); err != nil {
			return err
		}
	}
	if portfolioWatch {
		return holdings.Watch(ctx, appUI, loader, book)
	}
	holdings.Render(appUI, book)
	return nil
}

func init() {
	portfolioCmd.Flags().StringSliceVar(&portfolioChains, "chain", nil, "network name or chain id to include (repeatable). Combines with -k/--network. With neither, every chain is included")
	portfolioCmd.Flags().Float64Var(&portfolioMinUSD, "min-usd", 1, "hide holdings worth less than this many USD")
	portfolioCmd.Flags().Float64Var(&portfolioExitMin, "exit-min", 1000, "quote an exit tier for holdings at or above this many USD")
	portfolioCmd.Flags().BoolVar(&portfolioWatch, "watch", false, "follow live prices, updating changed lines in place")
	portfolioCmd.Flags().StringVarP(&portfolioJSON, "json-output", "o", "", "write the portfolio to a JSON file")
	rootCmd.AddCommand(portfolioCmd)
}

// selectPortfolioChains turns -k/--network and --chain into the chain
// set the loader keeps. An unset -k keeps its cobra default and does not
// narrow the book; an explicit -k is one more chain, unioned with --chain.
func selectPortfolioChains(networkSet bool, network string, chains []string) (map[int]struct{}, error) {
	values := append([]string{}, chains...)
	if networkSet {
		values = append(values, network)
	}
	return parsePortfolioChains(values)
}

func parsePortfolioChains(values []string) (map[int]struct{}, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := map[int]struct{}{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if id, err := strconv.Atoi(v); err == nil && id > 0 {
			out[id] = struct{}{}
			continue
		}
		n, err := networks.GetNetwork(v)
		if err != nil {
			return nil, fmt.Errorf("unknown chain %q", v)
		}
		out[int(n.GetChainID())] = struct{}{}
	}
	return out, nil
}
