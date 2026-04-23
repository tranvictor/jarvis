package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/walletconnect"
	"github.com/tranvictor/jarvis/walletconnect/gateways"
)

// wc command flag-bound package-level vars.
var (
	wcOwner     string
	wcProjectID string
	wcRelayURL  string
	wcVerbose   bool
)

// jarvisWCProjectIDEnv is the environment variable operators can set
// to avoid passing --project-id on every invocation.
const jarvisWCProjectIDEnv = "JARVIS_WC_PROJECT_ID"

// jarvisWCVerboseEnv enables [wc:debug] bridge tracing when set to
// 1, true, or yes.
const jarvisWCVerboseEnv = "JARVIS_WC_VERBOSE"

// jarvisWCDefaultProjectID is the fallback projectId jarvis uses when
// the user neither passes --project-id nor sets JARVIS_WC_PROJECT_ID.
// It is a real, registered Reown projectId bundled with the CLI so
// the out-of-the-box experience "just works"; power users who hit the
// shared quota or want isolated analytics should register their own
// at https://cloud.reown.com and override via the flag or env var.
// The value is not a secret — Reown treats projectIds as user-agent
// tokens for quota accounting, not as authentication material — so
// shipping it in-source is fine.
const jarvisWCDefaultProjectID = "0b7e16713eff42daf7861f5c223fda69"

// wcCmd turns jarvis into a WalletConnect v2 wallet for the duration
// of one command. See walletconnect/doc.go for the full design; the
// short version is:
//
//	jarvis wc <wc:uri> --from <account>
//
// blocks until the dApp disconnects or the user hits Ctrl-C.
var wcCmd = &cobra.Command{
	Use:   "wc <wc-uri>",
	Short: "Pair with a dApp as an EOA / Safe / Classic multisig over WalletConnect v2",
	Long: `Pair with a browser-hosted dApp (AAVE, KyberSwap, Uniswap, ...) using the
WalletConnect v2 protocol and service its eth_sendTransaction /
personal_sign / eth_signTypedData_v4 / wallet_switchEthereumChain
requests using a local jarvis-known account.

The command is blocking: jarvis pairs with the dApp using the URI
shown in its "Connect Wallet" dialog, prompts you to confirm the
session, and then stays connected — asking for per-request
confirmation — until you press Ctrl-C or the dApp disconnects.

The wallet kind is derived from --from:

  --from <eoa-addr|name>      : classic jarvis EOA (keystore / HW)
  --from <gnosis-safe-addr>   : Gnosis Safe multisig; jarvis proposes
                                a SafeTx per eth_sendTransaction
  --from <gnosis-classic-msig>: Legacy Gnosis MultiSigWallet; jarvis
                                broadcasts submitTransaction

For multisig accounts, jarvis auto-picks the local wallet that is
also one of the multisig's on-chain owners. If you have several
owner wallets locally, pass --owner to disambiguate.

The WalletConnect relay tags every client with a Reown projectId for
quota accounting. Jarvis ships with a bundled default so the command
works out of the box; if you hit relay rate limits, register your own
at https://cloud.reown.com and override via --project-id or
JARVIS_WC_PROJECT_ID.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runWC(args[0])
	},
}

func runWC(uri string) {
	projectID := wcProjectID
	if projectID == "" {
		projectID = os.Getenv(jarvisWCProjectIDEnv)
	}
	if projectID == "" {
		projectID = jarvisWCDefaultProjectID
	}
	verbose := wcVerbose
	if !verbose {
		verbose = envTruthy(os.Getenv(jarvisWCVerboseEnv))
	}
	if config.From == "" {
		appUI.Error("--from is required; pass an EOA address/name or a multisig address.")
		os.Exit(2)
	}

	network := config.Network()
	resolver := cmdutil.DefaultABIResolver{}

	// Ctrl-C cleanly tears the session down: we want jarvis to tell
	// the dApp we're leaving (wc_sessionDelete) rather than just
	// pulling the plug, so the dApp UI updates immediately instead
	// of waiting for its session-expiry timer.
	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	sess, err := walletconnect.Open(ctx, walletconnect.OpenConfig{
		URI:       uri,
		From:      config.From,
		Owner:     wcOwner,
		Network:   network,
		Resolver:  resolver,
		UI:        appUI,
		ProjectID: projectID,
		RelayURL:  wcRelayURL,
		Factory:   buildGateway,
		Verbose:   verbose,
	})
	if err != nil {
		appUI.Error("%s", err)
		os.Exit(1)
	}

	if err := sess.Run(ctx); err != nil {
		if ctx.Err() != nil {
			appUI.Warn("Session ended: %s", ctx.Err())
			return
		}
		appUI.Error("Session ended with error: %s", err)
		os.Exit(1)
	}
	appUI.Success("Session ended cleanly.")
}

// buildGateway is the GatewayFactory jarvis ships with. Lives in the
// cmd package (not walletconnect) so walletconnect stays independent
// of the concrete gateway implementations and of cmd/util.
func buildGateway(
	classification walletconnect.Classification,
	ownerAcc jtypes.AccDesc,
	network jarvisnetworks.Network,
	resolver cmdutil.ABIResolver,
	ui walletconnect.UI,
) (walletconnect.Gateway, error) {
	switch classification.Kind {
	case walletconnect.KindEOA:
		return gateways.NewEOAGateway(ui, ownerAcc, network, resolver)
	case walletconnect.KindSafe:
		return gateways.NewSafeGateway(ui, ownerAcc, classification.Address, network, resolver)
	case walletconnect.KindClassic:
		return gateways.NewClassicGateway(ui, ownerAcc, classification.Address, network, resolver)
	}
	return nil, fmt.Errorf("buildGateway: unsupported kind %q", classification.Kind)
}

func envTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func init() {
	wcCmd.Flags().StringVarP(&config.From, "from", "f", "",
		"Account to act as: a jarvis wallet name/address, a Gnosis Safe address, or a classic multisig address.")
	wcCmd.Flags().StringVar(&wcOwner, "owner", "",
		"For multisig --from: the specific local owner wallet to sign with (optional if only one local wallet is an owner).")
	wcCmd.Flags().StringVar(&wcProjectID, "project-id", "",
		"WalletConnect projectId. Defaults to the bundled jarvis projectId; override with this flag or the "+jarvisWCProjectIDEnv+" env var if you hit relay rate limits. Register your own at https://cloud.reown.com.")
	wcCmd.Flags().StringVar(&wcRelayURL, "relay-url", "",
		"Override the relay URL (default: "+walletconnect.DefaultRelayURL+")")
	wcCmd.Flags().BoolVar(&wcVerbose, "verbose", false,
		"Print [wc:debug] lines: relay topic/tag, subscribe ids, ECDH topic, each encrypted frame summary (also "+jarvisWCVerboseEnv+"=1).")
	wcCmd.MarkFlagRequired("from")

	rootCmd.AddCommand(wcCmd)
}
