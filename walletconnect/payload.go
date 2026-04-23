package walletconnect

import "encoding/json"

// Session-layer payload types. These live on top of the relay's
// JSON-RPC envelope and are themselves wrapped in WC envelopes before
// transport.
//
// JSON shape follows @walletconnect/types Session* definitions.

// relayDesc describes a relay protocol inside a proposal/settle. Only
// Protocol is load-bearing ("irn"); Data is echoed from the dApp.
type relayDesc struct {
	Protocol string `json:"protocol"`
	Data     string `json:"data,omitempty"`
}

// AppMetadata is the "who am I" card each peer publishes. We show this
// to the operator at pair-time so they can reject obviously-wrong
// pairings ("this is supposed to be AAVE but the URL says example.com").
type AppMetadata struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	URL         string   `json:"url,omitempty"`
	Icons       []string `json:"icons,omitempty"`
}

// Namespace is the per-namespace capability announcement inside a
// proposal or settle. Chains + Methods + Events are all CAIP-2 /
// JSON-RPC strings. Accounts is only populated in settles (the
// wallet's side of the handshake).
type Namespace struct {
	Chains   []string `json:"chains,omitempty"`
	Methods  []string `json:"methods,omitempty"`
	Events   []string `json:"events,omitempty"`
	Accounts []string `json:"accounts,omitempty"`
}

// wcProposalParams is the body of wc_sessionPropose.
type wcProposalParams struct {
	Relays []relayDesc `json:"relays"`

	Proposer struct {
		PublicKey string      `json:"publicKey"`
		Metadata  AppMetadata `json:"metadata"`
	} `json:"proposer"`

	RequiredNamespaces map[string]Namespace `json:"requiredNamespaces"`
	OptionalNamespaces map[string]Namespace `json:"optionalNamespaces,omitempty"`

	PairingTopic    string `json:"pairingTopic,omitempty"`
	ExpiryTimestamp int64  `json:"expiryTimestamp,omitempty"`

	SessionProperties map[string]string `json:"sessionProperties,omitempty"`
}

// wcProposalResult is what we send back on the pairing topic after
// accepting a proposal. responderPublicKey lets the dApp derive the
// session symKey via ECDH.
type wcProposalResult struct {
	Relay              relayDesc `json:"relay"`
	ResponderPublicKey string    `json:"responderPublicKey"`
}

// wcSettleParams is the body of wc_sessionSettle — the wallet's
// promise to the dApp of what capabilities it'll honour on this
// session. `expiry` is unix seconds.
type wcSettleParams struct {
	Relay      relayDesc            `json:"relay"`
	Controller controllerDesc       `json:"controller"`
	Namespaces map[string]Namespace `json:"namespaces"`
	Expiry     int64                `json:"expiry"`

	SessionProperties map[string]string `json:"sessionProperties,omitempty"`
}

type controllerDesc struct {
	PublicKey string      `json:"publicKey"`
	Metadata  AppMetadata `json:"metadata"`
}

// wcSessionRequestParams is the body of wc_sessionRequest, the
// message that actually carries dApp RPC calls (eth_sendTransaction,
// personal_sign, ...).
type wcSessionRequestParams struct {
	ChainID string `json:"chainId"`
	Request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	} `json:"request"`
}

// wcSessionDeleteParams is sent when either peer hangs up. code/message
// follow @walletconnect/utils/getSdkError shape.
type wcSessionDeleteParams struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// wcSessionEventParams is the wallet → dApp notification used to
// advertise chainChanged / accountsChanged after
// wallet_switchEthereumChain.
type wcSessionEventParams struct {
	ChainID string `json:"chainId"`
	Event   struct {
		Name string      `json:"name"`
		Data interface{} `json:"data"`
	} `json:"event"`
}

// ethSendTxPayload is the wire form of the first arg of
// eth_sendTransaction. Numeric fields are hex-encoded with optional
// 0x prefix; we parse them tolerantly.
type ethSendTxPayload struct {
	From                 string `json:"from"`
	To                   string `json:"to,omitempty"`
	Gas                  string `json:"gas,omitempty"`
	GasPrice             string `json:"gasPrice,omitempty"`
	MaxFeePerGas         string `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas,omitempty"`
	Value                string `json:"value,omitempty"`
	Data                 string `json:"data,omitempty"`
	Input                string `json:"input,omitempty"` // some dApps use "input" instead of "data"
	Nonce                string `json:"nonce,omitempty"`
}

// switchChainPayload is the first arg of wallet_switchEthereumChain.
type switchChainPayload struct {
	ChainID string `json:"chainId"` // "0x<hex>"
}
