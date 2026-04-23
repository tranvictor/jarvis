package walletconnect

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tranvictor/jarvis/walletconnect/relay"
	"github.com/tranvictor/jarvis/walletconnect/wccrypto"
)

// sessionExpirySeconds is how long we promise to keep a session open
// if left undisturbed. WC clients normalise to 7 days; we match so
// dApps don't show "expires in minutes" warnings.
const sessionExpirySeconds int64 = 7 * 24 * 60 * 60

// jarvisMetadata is the identity we present to dApps. URL is a
// deliberate localhost-style marker because jarvis runs locally and
// has no web landing page — dApps that require a valid https URL here
// will reject the session, which is acceptable: those dApps don't
// work with CLI wallets in practice.
var jarvisMetadata = AppMetadata{
	Name:        "jarvis",
	Description: "jarvis WalletConnect gateway",
	URL:         "https://github.com/tranvictor/jarvis",
	Icons:       nil,
}

// UI is the (small) interface Session uses to talk to the terminal:
// progress prints, confirm prompts, and structured errors. Injected
// so the session layer doesn't pull in jarvis/ui directly — keeps it
// unit-testable and avoids an import cycle.
type UI interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
	Success(format string, args ...interface{})
	Section(title string)
	Confirm(prompt string, defaultYes bool) bool
}

// Session represents one live WalletConnect pairing+session. It owns
// the relay client, the session symKey, and the Gateway; Run blocks
// until the peer hangs up, the context is cancelled, or the relay
// dies.
//
// Not safe for concurrent use across different Run() invocations; one
// Session == one command invocation.
type Session struct {
	ui      UI
	gw      Gateway
	relay   *relay.Client
	cryptoKP *wccrypto.KeyPair
	pairingTopic string
	pairingSymKey [32]byte
	sessionTopic string
	sessionSymKey [32]byte

	// Namespaces we've agreed to honour, indexed by namespace name.
	// Populated at settle time; consulted for every incoming request.
	namespaces map[string]Namespace

	// Metadata of the dApp we paired with.
	peerMeta AppMetadata

	// Verbose turns on [wc:debug] tracing (relay deliveries, subscribe
	// ids, crypto fingerprints, wire JSON-RPC). Set via
	// `jarvis wc --verbose` or JARVIS_WC_VERBOSE=1.
	verbose bool

	nextRequestID uint64

	// proposeRespID is the JSON-RPC id of the wc_sessionPropose we
	// just answered. Since we write our approval as a type-0 envelope
	// on the pairing topic (same as the original request), the relay
	// echoes our own frame back to us. We use this to ignore that
	// echo during settle-wait. Zero means "not set yet".
	proposeRespID uint64

	// redial rebuilds a fresh relay.Client with a new auth JWT.
	// Populated by Open() so the session can transparently
	// reconnect after a relay-initiated close (e.g. 4010 load
	// balancing, 45s idle drop).
	redial func(context.Context) (*relay.Client, error)
}

// RelayDialer is a function that produces a fresh *relay.Client. The
// session invokes it on reconnect with a short-lived context. Kept
// here (rather than in open.go) because Session is what actually owns
// the reconnect lifecycle.
type RelayDialer func(context.Context) (*relay.Client, error)

// SetRelayDialer registers a function used to rebuild the relay after
// a dropped connection. Optional; if unset, the session fails hard on
// a relay close just like before.
func (s *Session) SetRelayDialer(d RelayDialer) { s.redial = d }

// SessionConfig bundles the inputs Session.Run needs.
type SessionConfig struct {
	UI      UI
	URI     *URI
	Gateway Gateway
	Relay   *relay.Client
	KeyPair *wccrypto.KeyPair
	// Verbose enables dbg logging (see Session.dbg).
	Verbose bool
}

// NewSession wires up a Session but does not do any I/O; the caller
// calls Run next.
func NewSession(cfg SessionConfig) (*Session, error) {
	if cfg.URI == nil {
		return nil, fmt.Errorf("SessionConfig.URI is required")
	}
	if cfg.Gateway == nil {
		return nil, fmt.Errorf("SessionConfig.Gateway is required")
	}
	if cfg.Relay == nil {
		return nil, fmt.Errorf("SessionConfig.Relay is required")
	}
	if cfg.KeyPair == nil {
		return nil, fmt.Errorf("SessionConfig.KeyPair is required")
	}
	if cfg.UI == nil {
		return nil, fmt.Errorf("SessionConfig.UI is required")
	}
	var pairingSym [32]byte
	copy(pairingSym[:], cfg.URI.SymKey)
	return &Session{
		ui:            cfg.UI,
		gw:            cfg.Gateway,
		relay:         cfg.Relay,
		cryptoKP:      cfg.KeyPair,
		pairingTopic:  cfg.URI.Topic,
		pairingSymKey: pairingSym,
		verbose:       cfg.Verbose,
	}, nil
}

// Run drives the session state machine:
//
//  1. subscribe on pairingTopic
//  2. wait for wc_sessionPropose, confirm with operator, respond
//  3. subscribe on sessionTopic, send wc_sessionSettle
//  4. loop handling wc_sessionRequest / wc_sessionPing /
//     wc_sessionDelete until ctx is cancelled or relay disconnects.
//
// The only exit-without-error path is ctx cancellation or a clean
// wc_sessionDelete from the peer.
func (s *Session) Run(ctx context.Context) error {
	s.dbg("pairing topic: %s", s.pairingTopic)
	if sub, err := s.relay.Subscribe(ctx, s.pairingTopic); err != nil {
		return fmt.Errorf("subscribe pairing topic: %w", err)
	} else {
		s.dbg("irn_subscribe(pairing) ok subscriptionId=%s", sub)
	}
	s.ui.Info("Paired. Waiting for session proposal from dApp...")

	if err := s.awaitAndAcceptProposal(ctx); err != nil {
		return err
	}

	// Inform operator the session is live.
	s.ui.Success("Session established with %s", s.peerMeta.displayName())
	s.ui.Info("Press Ctrl-C to disconnect.")

	return s.runWithReconnect(ctx)
}

// runWithReconnect drives requestLoop, transparently reconnecting to
// the relay when the close looks recoverable (load balancing, idle
// drop, transient network blip). On ctx cancellation or user-
// initiated disconnect the loop returns like normal.
func (s *Session) runWithReconnect(ctx context.Context) error {
	const (
		maxReconnects     = 8
		baseBackoff       = 1 * time.Second
		maxBackoff        = 30 * time.Second
	)
	attempt := 0
	for {
		err := s.requestLoop(ctx)
		if err == nil {
			return nil
		}
		// Context cancellation always wins — the operator is done.
		if ctx.Err() != nil {
			return err
		}
		if s.redial == nil || !isRecoverableRelayClose(err) {
			return err
		}
		if attempt >= maxReconnects {
			return fmt.Errorf("relay kept dropping (%d reconnects exhausted): %w", attempt, err)
		}
		backoff := baseBackoff << attempt
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		attempt++
		s.ui.Warn("Relay dropped the connection (%s). Reconnecting in %s (attempt %d/%d)...",
			err, backoff, attempt, maxReconnects)
		s.dbg("runWithReconnect: relay closed (%s); redialing after %s (attempt %d/%d)",
			err, backoff, attempt, maxReconnects)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		if rerr := s.reconnect(ctx); rerr != nil {
			s.ui.Warn("Reconnect failed: %s", rerr)
			s.dbg("runWithReconnect: reconnect err: %s", rerr)
			// Loop back; requestLoop will immediately bail on the
			// stale client, triggering another backoff round.
			continue
		}
		s.ui.Success("Relay reconnected. Resuming session.")
		attempt = 0
	}
}

// reconnect rebuilds the relay client (fresh JWT) and re-subscribes to
// both pairing + session topics so pending requests from the dApp
// aren't lost. The pairing/session symKeys and topics are unchanged,
// so the dApp keeps using the same encryption context.
func (s *Session) reconnect(ctx context.Context) error {
	if s.redial == nil {
		return fmt.Errorf("no relay dialer registered")
	}
	// Close any lingering state from the old connection. Close is
	// idempotent and safe on an already-closed client.
	if s.relay != nil {
		_ = s.relay.Close()
	}
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rc, err := s.redial(dialCtx)
	if err != nil {
		return fmt.Errorf("redial relay: %w", err)
	}
	s.relay = rc

	if _, err := s.relay.Subscribe(ctx, s.pairingTopic); err != nil {
		return fmt.Errorf("resubscribe pairing topic: %w", err)
	}
	if s.sessionTopic != "" {
		if _, err := s.relay.Subscribe(ctx, s.sessionTopic); err != nil {
			return fmt.Errorf("resubscribe session topic: %w", err)
		}
	}
	return nil
}

// isRecoverableRelayClose reports whether the error returned by the
// relay read loop is worth reconnecting on. Matches close code 4010
// (load balancing), 1000/1001 (normal / going away — the relay
// occasionally cycles nodes), and transient network errors.
func isRecoverableRelayClose(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"close 4010",             // reown relay load-balancing rotation
		"close 1001",             // going away
		"close 1006",             // abnormal close
		"close 1011",             // server error
		"disconnecting for load balancing",
		"use of closed network connection",
		"connection reset by peer",
		"broken pipe",
		"i/o timeout",
		"unexpected eof",
		"eof",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// awaitAndAcceptProposal blocks until we receive a wc_sessionPropose
// on the pairing topic. It prompts the operator to accept or reject,
// responds on the pairing topic, and sends the settle message on the
// derived session topic.
func (s *Session) awaitAndAcceptProposal(ctx context.Context) error {
	proposeID, proposal, err := s.readProposal(ctx)
	if err != nil {
		return err
	}
	s.peerMeta = proposal.Proposer.Metadata

	s.ui.Section("dApp wants to connect")
	s.ui.Info("Name       : %s", proposal.Proposer.Metadata.Name)
	if proposal.Proposer.Metadata.URL != "" {
		s.ui.Info("URL        : %s", proposal.Proposer.Metadata.URL)
	}
	if proposal.Proposer.Metadata.Description != "" {
		s.ui.Info("Description: %s", proposal.Proposer.Metadata.Description)
	}
	s.printNamespaces("Required", proposal.RequiredNamespaces)
	s.printNamespaces("Optional", proposal.OptionalNamespaces)

	// Decide the final namespaces we'll promise. For every eip155
	// namespace we intersect the proposed chains/methods with what
	// the gateway supports, and fill in accounts based on the
	// gateway's CAIP-10.
	finalNS, err := s.buildNamespaces(ctx, proposal)
	if err != nil {
		s.sendProposalError(ctx, proposeID, 5100, err.Error())
		return fmt.Errorf("incompatible proposal: %w", err)
	}

	// No interactive approval prompt here: the operator already opted
	// in by launching `jarvis wc <uri> --from ...`. The metadata
	// above is printed purely so the user can sanity-check the dApp
	// they're pairing with; if anything looks wrong they can Ctrl-C
	// before the first request arrives.
	s.ui.Info(
		"Pairing as %s (%s). Press Ctrl-C at any time to end the session.",
		s.gw.Kind(), s.gw.Account(),
	)

	// Derive session key + topic via ECDH(peerPub, ourPriv).
	var peerPub [32]byte
	if err := hexDecodeInto(proposal.Proposer.PublicKey, peerPub[:]); err != nil {
		return fmt.Errorf("decode proposer publicKey: %w", err)
	}
	sessKey, sessTopic, err := wccrypto.DeriveSessionSymKey(s.cryptoKP.Private, peerPub)
	if err != nil {
		return fmt.Errorf("derive session symKey: %w", err)
	}
	s.sessionSymKey = sessKey
	s.sessionTopic = sessTopic
	s.namespaces = finalNS
	s.dbg("ecdh: %s  %s  sessionTopic=%s", redactKey32("symKey=", sessKey), redactKey32("walletPub=", s.cryptoKP.Public), sessTopic)
	s.dbg("namespace settle JSON size: %d bytes (approx)", mustJSONSize(finalNS))

	// Send propose response (type-1 envelope, because the dApp needs
	// our pubkey to derive the same symKey). Echo the dApp's relay
	// entry if present; some front-ends (Kyber, etc.) use relay.data
	// for their pending-session correlation.
	relay := relayDesc{Protocol: "irn"}
	if len(proposal.Relays) > 0 {
		relay = proposal.Relays[0]
	}
	result := wcProposalResult{
		Relay:              relay,
		ResponderPublicKey: hex.EncodeToString(s.cryptoKP.Public[:]),
	}
	if err := s.sendProposeApproveResponse(ctx, s.pairingTopic, proposeID, result,
		tagSessionProposeResponse, ttlFiveMin); err != nil {
		return fmt.Errorf("send propose response: %w", err)
	}
	s.proposeRespID = proposeID
	s.dbg("sent wc_sessionPropose approve result on pairing topic; propose json-rpc id=%d tag=%d", proposeID, tagSessionProposeResponse)

	// Subscribe to session topic BEFORE sending settle so we don't
	// miss the settle ack.
	if sub, err := s.relay.Subscribe(ctx, s.sessionTopic); err != nil {
		return fmt.Errorf("subscribe session topic: %w", err)
	} else {
		s.dbg("irn_subscribe(session) ok subscriptionId=%s topic=%s", sub, s.sessionTopic)
	}

	// Send wc_sessionSettle.
	settleRelay := relayDesc{Protocol: "irn"}
	if len(proposal.Relays) > 0 {
		settleRelay = proposal.Relays[0]
	}
	settleParams := wcSettleParams{
		Relay: settleRelay,
		Controller: controllerDesc{
			PublicKey: hex.EncodeToString(s.cryptoKP.Public[:]),
			Metadata:  jarvisMetadata,
		},
		Namespaces: finalNS,
		Expiry:     time.Now().Unix() + sessionExpirySeconds,
	}
	if len(proposal.SessionProperties) > 0 {
		settleParams.SessionProperties = proposal.SessionProperties
	}
	s.dbg("wc_sessionSettle payload size %d bytes; tag=%d", mustJSONSize(settleParams), tagSessionSettleRequest)
	settleReqID, err := s.sendRequest(ctx, s.sessionTopic, wcMethodSessionSettle, settleParams,
		tagSessionSettleRequest, ttlFiveMin)
	if err != nil {
		return fmt.Errorf("send settle: %w", err)
	}
	s.dbg("published wc_sessionSettle; json-rpc id=%d (wait for tag=%d response)", settleReqID, tagSessionSettleResponse)
	if err := s.waitSettleAck(ctx, settleReqID); err != nil {
		return err
	}
	s.dbg("waitSettleAck: got result true, session ready")
	return nil
}

// readProposal waits for exactly one wc_sessionPropose delivery on
// the pairing topic.
func (s *Session) readProposal(ctx context.Context) (uint64, *wcProposalParams, error) {
	for {
		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case del, ok := <-s.relay.Incoming():
			if !ok {
				return 0, nil, fmt.Errorf("relay closed before proposal: %w", s.relay.Err())
			}
			s.logDelivery("readProposal", del)
			if !strings.EqualFold(del.Topic, s.pairingTopic) {
				s.dbg("readProposal: skip (want pairing topic %s)", s.pairingTopic)
				// Stray delivery — shouldn't happen pre-settle.
				continue
			}
			out, err := wccrypto.Decrypt(s.pairingSymKey, del.Message)
			if err != nil {
				s.dbg("readProposal: decrypt type-0 fail: %s", err)
				s.ui.Warn("Failed to decrypt pairing-topic message: %s", err)
				continue
			}
			s.dbg("readProposal: decrypted %d bytes", len(out.Plaintext))
			s.dbg("readProposal: raw plaintext: %s", truncStr(string(out.Plaintext), 1500))
			w, err := decodeWireRPC(out.Plaintext)
			if err != nil {
				return 0, nil, fmt.Errorf("parse pairing frame: %w", err)
			}
			s.dbg("readProposal: %s", summaryWire(w))
			if w.Method != string(wcMethodSessionPropose) {
				s.ui.Warn("ignoring unexpected pairing method %q", w.Method)
				continue
			}
			propID, err := idUint64FromRaw(w.ID)
			if err != nil {
				return 0, nil, fmt.Errorf("propose id: %w", err)
			}
			var p wcProposalParams
			if err := json.Unmarshal(w.Params, &p); err != nil {
				return 0, nil, fmt.Errorf("decode propose params: %w", err)
			}
			return propID, &p, nil
		}
	}
}

// buildNamespaces decides which (chain, method, event) tuples jarvis
// will service on this session. We only speak eip155 — anything else
// in requiredNamespaces is fatal; in optionalNamespaces it's dropped.
//
// Chain selection: the gateway advertises its supported chains;
// intersect with proposed. If required chains aren't a subset, error.
// Method selection: intersect too, again requiring required to be a
// subset. Accounts: the gateway's CAIP-10, once per chain.
func (s *Session) buildNamespaces(
	ctx context.Context, p *wcProposalParams,
) (map[string]Namespace, error) {
	gatewayChains, err := s.gw.Chains(ctx)
	if err != nil {
		return nil, fmt.Errorf("gateway.Chains: %w", err)
	}
	gatewayChainSet := sliceSet(gatewayChains)
	methodSet := sliceSet(s.gw.Methods())

	out := map[string]Namespace{}

	mergeInto := func(dst *Namespace, src Namespace, required bool) error {
		for _, ch := range src.Chains {
			if _, ok := gatewayChainSet[ch]; !ok {
				if required {
					return fmt.Errorf(
						"required chain %s is not supported by the %s gateway",
						ch, s.gw.Kind())
				}
				continue
			}
			if !contains(dst.Chains, ch) {
				dst.Chains = append(dst.Chains, ch)
			}
		}
		for _, m := range src.Methods {
			if _, ok := methodSet[m]; !ok {
				if required {
					return fmt.Errorf(
						"required method %s is not supported by the %s gateway",
						m, s.gw.Kind())
				}
				continue
			}
			if !contains(dst.Methods, m) {
				dst.Methods = append(dst.Methods, m)
			}
		}
		for _, e := range src.Events {
			if !contains(dst.Events, e) {
				dst.Events = append(dst.Events, e)
			}
		}
		return nil
	}

	for ns, spec := range p.RequiredNamespaces {
		if ns != "eip155" {
			return nil, fmt.Errorf(
				"namespace %q is not supported (jarvis only handles eip155)", ns)
		}
		cur := out[ns]
		if err := mergeInto(&cur, spec, true); err != nil {
			return nil, err
		}
		out[ns] = cur
	}
	for ns, spec := range p.OptionalNamespaces {
		if ns != "eip155" {
			continue
		}
		cur := out[ns]
		_ = mergeInto(&cur, spec, false)
		out[ns] = cur
	}
	// If neither required nor optional listed eip155, fall back to
	// just the gateway's own chain set + methods.
	cur := out["eip155"]
	if len(cur.Chains) == 0 {
		cur.Chains = gatewayChains
	}
	if len(cur.Methods) == 0 {
		cur.Methods = s.gw.Methods()
	}
	if len(cur.Events) == 0 {
		cur.Events = []string{"chainChanged", "accountsChanged"}
	}
	// Build accounts list from chains x gateway.Account().
	accts := make([]string, 0, len(cur.Chains))
	for _, ch := range cur.Chains {
		chainID, err := ParseChain(ch)
		if err != nil {
			continue
		}
		// gateway.Account() is already CAIP-10; rebuild per chain.
		_, addr, err := ParseAccount(s.gw.Account())
		if err == nil {
			accts = append(accts, AccountString(chainID, addr.Hex()))
		}
	}
	cur.Accounts = accts
	if len(cur.Accounts) == 0 {
		return nil, fmt.Errorf(
			"eip155 session would have no accounts (check --network and gateway account)")
	}
	out["eip155"] = cur
	return out, nil
}

// requestLoop pumps deliveries off the relay until ctx or the
// connection closes. Every request is dispatched to the right
// gateway method; every failure is reported back to the dApp with
// the mapped WC error code so their UI stops spinning.
func (s *Session) requestLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			s.sendSessionDelete(context.Background(), 6000, "user disconnected")
			return ctx.Err()
		case <-s.relay.Done():
			return fmt.Errorf("relay closed: %w", s.relay.Err())
		case del, ok := <-s.relay.Incoming():
			if !ok {
				return fmt.Errorf("relay closed: %w", s.relay.Err())
			}
			s.logDelivery("requestLoop", del)
			// We're only interested in sessionTopic deliveries
			// post-settle; ignore late messages on the pairing topic.
			if !strings.EqualFold(del.Topic, s.sessionTopic) {
				s.dbg("requestLoop: skip (want session topic %s)", s.sessionTopic)
				continue
			}
			out, err := wccrypto.Decrypt(s.sessionSymKey, del.Message)
			if err != nil {
				s.dbg("requestLoop: decrypt type-0 fail: %s", err)
				s.ui.Warn("Failed to decrypt session-topic message: %s", err)
				continue
			}
			s.dbg("requestLoop: decrypted %d bytes", len(out.Plaintext))
			s.dispatch(ctx, out.Plaintext)
		}
	}
}

// dispatch routes a single decrypted session frame. It runs
// synchronously — we don't want two prompts to race — which means the
// relay can back up while a user is deciding. That's acceptable given
// the manual nature of the UX.
func (s *Session) dispatch(ctx context.Context, raw []byte) {
	w, err := decodeWireRPC(raw)
	if err != nil {
		s.ui.Warn("malformed session frame: %s", err)
		return
	}
	s.dbg("dispatch: %s", summaryWire(w))
	rid, err := idUint64FromRaw(w.ID)
	if err != nil {
		s.ui.Warn("session frame id: %s", err)
		return
	}

	// Response to something WE sent (settle ack, event ack, etc.) —
	// nothing to do beyond logging if it's an error.
	if w.Method == "" {
		if w.Error != nil {
			s.ui.Warn("dApp returned error for our %d: %s", rid, w.Error.Message)
		}
		return
	}

	switch wcMethod(w.Method) {
	case wcMethodSessionRequest:
		s.handleSessionRequest(ctx, rid, w.Params)
	case wcMethodSessionPing:
		_ = s.sendResponseType0(ctx, s.sessionTopic, rid, true,
			tagSessionPingResponse, ttlThirty)
	case wcMethodSessionDelete:
		var d wcSessionDeleteParams
		_ = json.Unmarshal(w.Params, &d)
		s.ui.Warn("dApp ended session: %s (code %d)", d.Message, d.Code)
		_ = s.sendResponseType0(ctx, s.sessionTopic, rid, true,
			tagSessionDeleteResponse, ttlOneDay)
		_ = s.relay.Close()
	case wcMethodSessionUpdate, wcMethodSessionExtend:
		// Accept with a generic true; we don't change our promise
		// based on these.
		_ = s.sendResponseType0(ctx, s.sessionTopic, rid, true,
			tagSessionUpdateResponse, ttlOneDay)
	default:
		s.ui.Warn("unhandled WC method %q", w.Method)
		s.sendErrorResponse(ctx, s.sessionTopic, rid, 5101,
			"method not supported", tagSessionRequestResponse, ttlFiveMin)
	}
}

// handleSessionRequest unwraps the inner eth_* call, checks namespace
// compliance, and dispatches to the gateway.
func (s *Session) handleSessionRequest(ctx context.Context, id uint64, params json.RawMessage) {
	var req wcSessionRequestParams
	if err := json.Unmarshal(params, &req); err != nil {
		s.sendErrorResponse(ctx, s.sessionTopic, id, 5000,
			"malformed wc_sessionRequest", tagSessionRequestResponse, ttlFiveMin)
		return
	}

	// Namespace enforcement: chain must be in our settle's chains
	// and method in our methods.
	ns := s.namespaces["eip155"]
	if !contains(ns.Chains, req.ChainID) {
		s.sendErrorResponse(ctx, s.sessionTopic, id, 5100,
			fmt.Sprintf("chain %s not in session scope", req.ChainID),
			tagSessionRequestResponse, ttlFiveMin)
		return
	}
	if !contains(ns.Methods, req.Request.Method) {
		s.sendErrorResponse(ctx, s.sessionTopic, id, 5101,
			fmt.Sprintf("method %s not in session scope", req.Request.Method),
			tagSessionRequestResponse, ttlFiveMin)
		return
	}

	s.ui.Section(fmt.Sprintf("dApp request: %s on %s", req.Request.Method, req.ChainID))
	result, err := s.dispatchMethod(ctx, req.ChainID, req.Request.Method, req.Request.Params)
	if err != nil {
		code, msg := errorCodeFor(err)
		s.ui.Error("Request failed: %s", msg)
		s.sendErrorResponse(ctx, s.sessionTopic, id, code, msg,
			tagSessionRequestResponse, ttlFiveMin)
		return
	}
	if err := s.sendResponseType0(ctx, s.sessionTopic, id, result,
		tagSessionRequestResponse, ttlFiveMin); err != nil {
		s.ui.Error("Failed to send response to dApp: %s", err)
	}
}

func (s *Session) dispatchMethod(
	ctx context.Context, chain, method string, params json.RawMessage,
) (interface{}, error) {
	switch method {
	case SupportedMethods.SendTransaction:
		tx, err := parseSendTxParams(params)
		if err != nil {
			return nil, err
		}
		return s.gw.SendTransaction(ctx, chain, tx)
	case SupportedMethods.PersonalSign:
		msg, err := parsePersonalSignParams(params)
		if err != nil {
			return nil, err
		}
		return s.gw.PersonalSign(ctx, chain, msg)
	case SupportedMethods.SignTypedDataV4:
		td, err := parseSignTypedDataV4Params(params)
		if err != nil {
			return nil, err
		}
		return s.gw.SignTypedData(ctx, chain, td)
	case SupportedMethods.SwitchChain:
		targetID, err := parseSwitchChainParams(params)
		if err != nil {
			return nil, err
		}
		if err := s.gw.SwitchChain(ctx, ChainString(targetID)); err != nil {
			return nil, err
		}
		// Tell dApp so it can update its UI.
		s.emitChainChanged(ctx, targetID)
		// EIP-3326 return shape is null on success.
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrMethodNotSupported, method)
	}
}

// emitChainChanged sends a wc_sessionEvent notifying the dApp that
// accountsChanged/chainChanged now applies.
func (s *Session) emitChainChanged(ctx context.Context, chainID uint64) {
	ev := wcSessionEventParams{ChainID: ChainString(chainID)}
	ev.Event.Name = "chainChanged"
	ev.Event.Data = chainID
	_, _ = s.sendRequest(ctx, s.sessionTopic, wcMethodSessionEvent, ev,
		tagSessionEventRequest, ttlFiveMin)
}

// sendSessionDelete best-effort notifies the peer we're hanging up.
// Uses a short standalone timeout so it can run during shutdown.
func (s *Session) sendSessionDelete(ctx context.Context, code int, message string) {
	if s.sessionTopic == "" {
		return
	}
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, _ = s.sendRequest(c, s.sessionTopic, wcMethodSessionDelete,
		wcSessionDeleteParams{Code: code, Message: message},
		tagSessionDeleteRequest, ttlOneDay)
}

// -- low-level publish helpers ----------------------------------------------

// jsonrpcEnvelope is the unified JSON-RPC envelope we emit on every
// WC message.
type jsonrpcEnvelope struct {
	ID      uint64          `json:"id"`
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// sendRequest publishes a type-0 (symKey) WC request on topic.
func (s *Session) sendRequest(
	ctx context.Context, topic string, method wcMethod, params interface{},
	tag uint64, ttl time.Duration,
) (uint64, error) {
	rawParams, err := json.Marshal(params)
	if err != nil {
		return 0, err
	}
	id := s.newID()
	env := jsonrpcEnvelope{
		ID:      id,
		JSONRPC: "2.0",
		Method:  string(method),
		Params:  rawParams,
	}
	if body, merr := json.Marshal(env); merr == nil {
		s.dbg("sendRequest: method=%s id=%d tag=%d topic=%s plaintext=%s",
			method, id, tag, topic, truncStr(string(body), 900))
	}
	return id, s.publishType0(ctx, topic, env, tag, ttl, method == wcMethodSessionRequest)
}

// sendResponseType0 publishes a type-0 (symKey) JSON-RPC response.
func (s *Session) sendResponseType0(
	ctx context.Context, topic string, id uint64, result interface{},
	tag uint64, ttl time.Duration,
) error {
	var raw json.RawMessage
	if result != nil {
		b, err := json.Marshal(result)
		if err != nil {
			return err
		}
		raw = b
	} else {
		raw = json.RawMessage("null")
	}
	env := jsonrpcEnvelope{
		ID:      id,
		JSONRPC: "2.0",
		Result:  raw,
	}
	return s.publishType0(ctx, topic, env, tag, ttl, false)
}

// sendProposeApproveResponse publishes the wc_sessionPropose approval
// as a TYPE-0 envelope on the pairing topic, encrypted with the
// pairing symKey.
//
// Despite the responderPublicKey in the result looking like a type-1
// envelope would be used, the WalletConnect reference implementation
// (walletconnect-monorepo, sign-client engine `sendApproveSession`)
// encodes the pairing response with `crypto.encode(pairingTopic,
// payload, { encoding: BASE64 })` — i.e. the default TYPE_0 path using
// the pairing symKey. The `responderPublicKey` is a plaintext JSON
// field inside the result, not the type-1 senderPublicKey header byte.
//
// The dApp's pairing listener ignores TYPE_1 envelopes entirely (see
// `ignoredPayloadTypes = [TYPE_1]` in core/src/controllers/pairing.ts
// on WC v2). The engine's own listener rejects TYPE_1 messages
// whose decode options don't specify a receiverPublicKey. In practice
// the wallet ALWAYS uses TYPE_0 here.
func (s *Session) sendProposeApproveResponse(
	ctx context.Context, topic string, id uint64, result interface{},
	tag uint64, ttl time.Duration,
) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	env := jsonrpcEnvelope{
		ID:      id,
		JSONRPC: "2.0",
		Result:  raw,
	}
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	s.dbg("sendProposeApproveResponse: plaintext JSON (before encrypt): %s", truncStr(string(body), 900))
	s.dbg("sendProposeApproveResponse: using pairing symKey=%x (full)", s.pairingSymKey)
	encoded, err := wccrypto.Encrypt(s.pairingSymKey, body)
	if err != nil {
		return err
	}
	s.dbg("sendProposeApproveResponse: topic=%s tag=%d ciphertextLen=%d (type-0, pairing symKey)",
		topic, tag, len(encoded))
	return s.relay.Publish(ctx, topic, encoded, tag, ttl, false)
}

// sendProposalError writes a JSON-RPC error result on the pairing
// topic. Kept separate from sendErrorResponse because the pairing
// topic uses the pairing symKey (not the session symKey).
func (s *Session) sendProposalError(ctx context.Context, id uint64, code int, msg string) {
	env := jsonrpcEnvelope{
		ID:      id,
		JSONRPC: "2.0",
		Error:   &jsonrpcError{Code: code, Message: msg},
	}
	body, err := json.Marshal(env)
	if err != nil {
		return
	}
	encoded, err := wccrypto.Encrypt(s.pairingSymKey, body)
	if err != nil {
		return
	}
	_ = s.relay.Publish(ctx, s.pairingTopic, encoded, tagSessionProposeReject, ttlFiveMin, false)
}

// sendErrorResponse writes a JSON-RPC error reply on the session
// topic, encrypted with the session symKey.
func (s *Session) sendErrorResponse(
	ctx context.Context, topic string, id uint64, code int, msg string,
	tag uint64, ttl time.Duration,
) {
	env := jsonrpcEnvelope{
		ID:      id,
		JSONRPC: "2.0",
		Error:   &jsonrpcError{Code: code, Message: msg},
	}
	body, err := json.Marshal(env)
	if err != nil {
		return
	}
	var sym [32]byte
	if topic == s.sessionTopic {
		sym = s.sessionSymKey
	} else {
		sym = s.pairingSymKey
	}
	encoded, err := wccrypto.Encrypt(sym, body)
	if err != nil {
		return
	}
	_ = s.relay.Publish(ctx, topic, encoded, tag, ttl, false)
}

// publishType0 encrypts env with the right symmetric key for `topic`
// and publishes it via the relay with the given tag/ttl.
func (s *Session) publishType0(
	ctx context.Context, topic string, env jsonrpcEnvelope,
	tag uint64, ttl time.Duration, prompt bool,
) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	var sym [32]byte
	if topic == s.sessionTopic {
		sym = s.sessionSymKey
	} else {
		sym = s.pairingSymKey
	}
	encoded, err := wccrypto.Encrypt(sym, body)
	if err != nil {
		return err
	}
	return s.relay.Publish(ctx, topic, encoded, tag, ttl, prompt)
}

// newID returns a JSON-RPC id for session-layer messages. It mirrors
// @walletconnect/jsonrpc-utils `payloadId(entropy=3)` but with entropy
// clipped so the value stays below JavaScript's Number.MAX_SAFE_INTEGER
// (2^53-1 ≈ 9.007e15). Every other WC implementation uses
// `Date.now()*10^6 + rand(10^6)` (≈1.8e18) which actually exceeds safe
// integer precision and silently round-trips through JSON.parse →
// number → JSON.stringify in the browser, but that only works because
// *both* peers do the same rounding. We want to stay well within the
// safe range AND produce IDs that look like "real" payload ids to
// dApps that may bucket by magnitude.
//
// Using Date.now() * 1000 + random(1000) gives ~1.8e15 which is
// comfortably below 2^53 and is unique enough for a single session.
func (s *Session) newID() uint64 {
	// Monotonic counter inside the millisecond to guarantee uniqueness
	// even if the clock doesn't advance between calls.
	counter := atomic.AddUint64(&s.nextRequestID, 1) % 1000
	ms := uint64(time.Now().UnixMilli())
	return ms*1_000 + counter
}

// waitSettleAck blocks until the dApp answers our wc_sessionSettle with
// result true (or returns an error). Without this, we would print
// "Session established" while the browser was still waiting for a
// valid JSON-RPC round-trip.
//
// We also listen on the pairing topic for rejections: when a dApp can't
// decrypt our wc_sessionPropose response (e.g. a crypto mismatch) some
// implementations send a JSON-RPC error on the pairing topic using tag
// 1120/1121 so the wallet can distinguish "silent drop" from "I didn't
// like your approve". Surfacing any pairing-topic traffic here is what
// lets us give a useful error message instead of a generic 90s timeout.
func (s *Session) waitSettleAck(ctx context.Context, wantID uint64) error {
	s.ui.Info("Waiting for dApp to acknowledge the session (this can take 5-15s)...")
	s.dbg("waitSettleAck: start — want json-rpc id=%d on session topic=%s; also watching pairing topic=%s for rejections",
		wantID, s.sessionTopic, s.pairingTopic)
	waitCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()
	startedAt := time.Now()
	var deliveries int
	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				s.ui.Warn("No ack from dApp after 90s (saw %d total deliveries on session+pairing topics).", deliveries)
				s.dbg("waitSettleAck: timeout after 90s (no matching JSON-RPC response for id %d on topic %s)", wantID, s.sessionTopic)
				return fmt.Errorf(
					"timed out waiting for the dApp to confirm the session (wc_sessionSettle). " +
						"If the browser still shows the WalletConnect URI, try again or use your own Reown projectId")
			}
			return waitCtx.Err()
		case <-heartbeat.C:
			s.ui.Info("  ...still waiting (%.0fs, %d deliveries seen)",
				time.Since(startedAt).Seconds(), deliveries)
			s.dbg("waitSettleAck: still waiting... %.0fs elapsed; sessionTopic=%s pairingTopic=%s",
				time.Since(startedAt).Seconds(), s.sessionTopic, s.pairingTopic)
		case del, ok := <-s.relay.Incoming():
			if !ok {
				return fmt.Errorf("relay closed before session ready: %w", s.relay.Err())
			}
			deliveries++
			s.logDelivery("waitSettleAck", del)
			// Surface a one-liner even without --verbose so the
			// user can confirm traffic is flowing.
			s.ui.Info("  recv msg #%d  topic=%s…  tag=%d  bytes=%d",
				deliveries, shortTopic(del.Topic), del.Tag, len(del.Message))
			switch {
			case strings.EqualFold(del.Topic, s.sessionTopic):
				if done, err := s.handleSettleDelivery(waitCtx, del, wantID); done || err != nil {
					return err
				}
			case strings.EqualFold(del.Topic, s.pairingTopic):
				// The approve response may have been rejected on
				// the pairing topic. Try to decrypt with pairingSymKey
				// (type-0) and surface the payload.
				s.peekPairingDelivery(del)
			default:
				s.dbg("waitSettleAck: ignored delivery on unrelated topic %s (tag=%d, b64len=%d)",
					del.Topic, del.Tag, len(del.Message))
			}
		}
	}
}

// handleSettleDelivery processes a single delivery on the session
// topic. Returns (done=true) when the settle ack has been observed.
func (s *Session) handleSettleDelivery(
	ctx context.Context, del relay.Delivery, wantID uint64,
) (bool, error) {
	out, err := wccrypto.Decrypt(s.sessionSymKey, del.Message)
	if err != nil {
		s.dbg("waitSettleAck: session-topic decrypt fail: %s", err)
		s.ui.Warn("decrypt session frame while waiting for settle: %s", err)
		return false, nil
	}
	s.dbg("waitSettleAck: session-topic decrypted %d bytes: %s",
		len(out.Plaintext), truncStr(string(out.Plaintext), 900))
	w, err := decodeWireRPC(out.Plaintext)
	if err != nil {
		s.dbg("waitSettleAck: decodeWireRPC: %s", err)
		s.ui.Warn("parse session frame while waiting for settle: %s", err)
		return false, nil
	}
	s.dbg("waitSettleAck: %s", summaryWire(w))
	if w.Method == "" {
		got, err := idUint64FromRaw(w.ID)
		if err != nil {
			s.dbg("waitSettleAck: bad id field: %s (raw id json=%q)", err, string(w.ID))
			s.ui.Warn("settle-ack id: %s", err)
			return false, nil
		}
		if got != wantID {
			s.dbg("waitSettleAck: id mismatch (want %d, got %d) — still waiting", wantID, got)
			return false, nil
		}
		if w.Error != nil {
			return true, fmt.Errorf("dApp rejected session settlement: %s (code %d)", w.Error.Message, w.Error.Code)
		}
		if w.Result != nil && bytes.Equal(w.Result, []byte("true")) {
			return true, nil
		}
		return true, fmt.Errorf("unexpected wc_sessionSettle result: %s", string(w.Result))
	}
	rid, err := idUint64FromRaw(w.ID)
	if err != nil {
		s.dbg("waitSettleAck: pre-ack id parse: %s", err)
		s.ui.Warn("pre-ack request id: %s", err)
		return false, nil
	}
	if wcMethod(w.Method) == wcMethodSessionPing {
		s.dbg("waitSettleAck: replying to wc_sessionPing id=%d before settle ack", rid)
		_ = s.sendResponseType0(ctx, s.sessionTopic, rid, true,
			tagSessionPingResponse, ttlThirty)
		return false, nil
	}
	s.dbg("waitSettleAck: unexpected method %q (still waiting for settle ack)", w.Method)
	s.ui.Warn("unexpected session method before settle ack: %s (still waiting)", w.Method)
	return false, nil
}

// peekPairingDelivery tries to decrypt a pairing-topic message with
// the pairing symKey and logs the result. Used during settle-wait to
// catch JSON-RPC errors the dApp writes back to the pairing topic
// when our approve response was rejected.
//
// Our own propose-approve response is also a type-0 frame on the
// pairing topic (encrypted with the pairing symKey), so the relay
// will echo it back to us. We detect echoes by matching id == the
// JSON-RPC id of the wc_sessionPropose request we just responded to.
func (s *Session) peekPairingDelivery(del relay.Delivery) {
	out, err := wccrypto.Decrypt(s.pairingSymKey, del.Message)
	if err != nil {
		s.ui.Warn("pairing-topic frame (tag=%d) did not decrypt with pairing key: %s",
			del.Tag, err)
		s.dbg("waitSettleAck: pairing-topic decrypt fail (tag=%d): %s", del.Tag, err)
		return
	}
	s.dbg("waitSettleAck: pairing-topic plaintext (tag=%d): %s",
		del.Tag, truncStr(string(out.Plaintext), 900))
	w, err := decodeWireRPC(out.Plaintext)
	if err != nil {
		s.dbg("waitSettleAck: pairing-topic decode: %s", err)
		return
	}
	if w.Method == "" {
		// JSON-RPC response (no method). If id == proposeRespID it's
		// our own approve response echoed back by the relay — ignore.
		if got, err := idUint64FromRaw(w.ID); err == nil && got == s.proposeRespID {
			s.dbg("waitSettleAck: pairing-topic tag=%d is our own propose approve echoed back (id=%d); ignoring",
				del.Tag, got)
			return
		}
		if w.Error != nil {
			s.ui.Warn("[wc] dApp returned error on pairing topic (%d: %s). "+
				"Common cause: the dApp could not decrypt our wc_sessionPropose response.",
				w.Error.Code, w.Error.Message)
		}
		s.ui.Info("  pairing-topic plaintext: %s", truncStr(string(out.Plaintext), 300))
		return
	}
	s.ui.Info("  pairing-topic request method=%s (unexpected; still waiting on session)", w.Method)
}

// printNamespaces lists a map[ns]->Namespace as a readable block.
func (s *Session) printNamespaces(label string, nss map[string]Namespace) {
	if len(nss) == 0 {
		return
	}
	s.ui.Info("%s namespaces:", label)
	for name, ns := range nss {
		s.ui.Info("  [%s]", name)
		if len(ns.Chains) > 0 {
			s.ui.Info("    chains : %s", strings.Join(ns.Chains, ", "))
		}
		if len(ns.Methods) > 0 {
			s.ui.Info("    methods: %s", strings.Join(ns.Methods, ", "))
		}
		if len(ns.Events) > 0 {
			s.ui.Info("    events : %s", strings.Join(ns.Events, ", "))
		}
	}
}

// errorCodeFor maps our sentinel errors onto WC JSON-RPC error codes.
// Falls back to 5000 (generic wallet error) for anything else.
func errorCodeFor(err error) (int, string) {
	switch {
	case errors.Is(err, ErrUserRejected):
		return 5000, err.Error()
	case errors.Is(err, ErrChainNotSupported):
		return 5100, err.Error()
	case errors.Is(err, ErrMethodNotSupported):
		return 5101, err.Error()
	case errors.Is(err, ErrSessionExpired):
		return 5200, err.Error()
	default:
		return 5000, err.Error()
	}
}

// -- small helpers ----------------------------------------------------------

func sliceSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func hexDecodeInto(s string, out []byte) error {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s) != len(out)*2 {
		return fmt.Errorf("expected %d hex chars, got %d", len(out)*2, len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	copy(out, b)
	return nil
}

func (m AppMetadata) displayName() string {
	if m.Name != "" {
		return m.Name
	}
	if m.URL != "" {
		return m.URL
	}
	return "<unknown dApp>"
}
