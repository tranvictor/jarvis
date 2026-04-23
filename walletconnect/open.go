package walletconnect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tranvictor/jarvis/accounts"
	jtypes "github.com/tranvictor/jarvis/accounts/types"
	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/msig"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/safe"
	"github.com/tranvictor/jarvis/walletconnect/relay"
	"github.com/tranvictor/jarvis/walletconnect/wccrypto"
)

// DefaultRelayURL is the public WalletConnect relay used by both the
// Reown cloud and the legacy walletconnect.com brand. Overridable via
// --relay-url on the CLI for self-hosted deployments.
const DefaultRelayURL = "wss://relay.walletconnect.org"

// GatewayFactory assembles a concrete Gateway from a Classification.
// Kept as a function so `cmd/wc.go` (and its imports) is the only
// place the gateway subpackage needs to be referenced — the
// walletconnect package itself stays free of the cmd/util -> gateway
// chain that would otherwise bloat the dependency graph.
type GatewayFactory func(
	classification Classification,
	ownerAcc jtypes.AccDesc,
	network jarvisnetworks.Network,
	resolver cmdutil.ABIResolver,
	ui UI,
) (Gateway, error)

// OpenConfig collects the inputs Open needs. Most fields are required;
// the exceptions (RelayURL, DialTimeout) have sensible defaults.
type OpenConfig struct {
	URI       string
	From      string // user-supplied; matches --from
	Owner     string // optional; used for multisig sessions to pick the signer

	Network  jarvisnetworks.Network
	Resolver cmdutil.ABIResolver
	UI       UI

	ProjectID string
	RelayURL  string

	Factory GatewayFactory

	DialTimeout time.Duration
	// Verbose enables [wc:debug] tracing for the WalletConnect session
	// (set via `jarvis wc --verbose` or JARVIS_WC_VERBOSE=1).
	Verbose bool
}

// Open performs the full WC pair-up sequence up to the point where the
// session is ready to Run: parse URI, classify From, pick an owner
// wallet if needed, build the gateway, dial the relay, subscribe, and
// hand the caller a live *Session.
//
// The returned Session keeps an internal reference to the relay
// Client; closing the Session also closes the relay.
func Open(ctx context.Context, cfg OpenConfig) (*Session, error) {
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf(
			"walletconnect: --project-id (or JARVIS_WC_PROJECT_ID) is required. " +
				"Register a free projectId at https://cloud.reown.com")
	}
	if cfg.Factory == nil {
		return nil, fmt.Errorf("walletconnect: Open needs a GatewayFactory")
	}
	uri, err := ParseURI(cfg.URI)
	if err != nil {
		return nil, fmt.Errorf("parse wc: uri: %w", err)
	}

	classification, err := ClassifyAccount(cfg.Resolver, cfg.Network, cfg.From)
	if err != nil {
		return nil, err
	}

	ownerAcc, err := pickOwner(classification, cfg.Owner, cfg.Network)
	if err != nil {
		return nil, err
	}

	gw, err := cfg.Factory(classification, ownerAcc, cfg.Network, cfg.Resolver, cfg.UI)
	if err != nil {
		return nil, fmt.Errorf("build gateway: %w", err)
	}

	// Relay auth — ephemeral ed25519 keypair signing a short-lived JWT.
	relayKP, err := wccrypto.NewRelayKeyPair()
	if err != nil {
		return nil, fmt.Errorf("relay keypair: %w", err)
	}
	aud := cfg.RelayURL
	if aud == "" {
		aud = DefaultRelayURL
	}
	// dialRelay returns a fresh *relay.Client. The session uses this
	// both for the initial connection and for transparent reconnects
	// after a relay-side drop (e.g. 4010 load-balancing). Each call
	// mints a new short-lived JWT so we don't reuse a token that the
	// relay has already rotated away from.
	dialRelay := func(dctx context.Context) (*relay.Client, error) {
		jwt, err := wccrypto.BuildRelayJWT(relayKP, aud, 24*time.Hour)
		if err != nil {
			return nil, fmt.Errorf("build relay jwt: %w", err)
		}
		return relay.Dial(dctx, relay.Config{
			URL:         aud,
			ProjectID:   cfg.ProjectID,
			AuthJWT:     jwt,
			DialTimeout: cfg.DialTimeout,
		})
	}

	rc, err := dialRelay(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect to relay: %w", err)
	}

	sessionKP, err := wccrypto.NewKeyPair()
	if err != nil {
		_ = rc.Close()
		return nil, fmt.Errorf("session keypair: %w", err)
	}

	sess, err := NewSession(SessionConfig{
		UI:      cfg.UI,
		URI:     uri,
		Gateway: gw,
		Relay:   rc,
		KeyPair: sessionKP,
		Verbose: cfg.Verbose,
	})
	if err != nil {
		_ = rc.Close()
		return nil, err
	}
	sess.SetRelayDialer(dialRelay)

	if cfg.Verbose {
		cfg.UI.Info("[wc:debug] verbose logging on (relay frames, ECDH topic, wire JSON-RPC)")
	}
	cfg.UI.Info("Gateway : %s (%s)", gw.Kind(), gw.Account())
	cfg.UI.Info("Relay   : %s", aud)
	cfg.UI.Info("Topic   : %s", uri.Topic)

	return sess, nil
}

// pickOwner resolves the wallet that will actually sign on this
// session. For KindEOA the owner is just the classified account. For
// multisig kinds we need one of the on-chain owners; if --owner was
// passed we validate it matches, otherwise we look for an
// exactly-one local wallet among the multisig's owners (the same
// semantics `jarvis send` uses).
func pickOwner(
	c Classification,
	ownerFlag string,
	network jarvisnetworks.Network,
) (jtypes.AccDesc, error) {
	if c.Kind == KindEOA {
		return c.AccDesc, nil
	}

	owners, err := readOwners(c, network)
	if err != nil {
		return jtypes.AccDesc{}, fmt.Errorf("read %s owners: %w", c.Kind, err)
	}

	if ownerFlag != "" {
		acc, err := accounts.GetAccount(ownerFlag)
		if err != nil {
			return jtypes.AccDesc{}, fmt.Errorf(
				"--owner %s: no such local wallet: %w", ownerFlag, err)
		}
		found := false
		for _, o := range owners {
			if strings.EqualFold(o, acc.Address) {
				found = true
				break
			}
		}
		if !found {
			return jtypes.AccDesc{}, fmt.Errorf(
				"--owner %s is not an owner of %s", acc.Address, c.Address)
		}
		return acc, nil
	}

	// Auto-pick: scan local wallets for exactly one match against
	// the on-chain owner set.
	var hit jtypes.AccDesc
	matches := 0
	for _, o := range owners {
		if acc, err := accounts.GetAccount(o); err == nil {
			hit = acc
			matches++
		}
	}
	switch matches {
	case 0:
		return jtypes.AccDesc{}, fmt.Errorf(
			"no local wallet is an owner of %s; add one with `jarvis wallet add` "+
				"or pass --owner explicitly",
			c.Address)
	case 1:
		return hit, nil
	default:
		return jtypes.AccDesc{}, fmt.Errorf(
			"%d local wallets are owners of %s; pass --owner to choose",
			matches, c.Address)
	}
}

// readOwners dispatches to safe/msig depending on the classified
// kind. Inlining this here (rather than pushing it into classify.go)
// keeps classify.go a pure read-only classifier.
func readOwners(c Classification, network jarvisnetworks.Network) ([]string, error) {
	switch c.Kind {
	case KindSafe:
		sc, err := safe.NewSafeContract(c.Address, network)
		if err != nil {
			return nil, err
		}
		return sc.Owners()
	case KindClassic:
		mc, err := msig.NewMultisigContract(c.Address, network)
		if err != nil {
			return nil, err
		}
		return mc.Owners()
	}
	return nil, fmt.Errorf("readOwners: unsupported kind %q", c.Kind)
}
