package safe

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

// SafeShortNameChainID maps the chain "short names" used by Safe app URLs
// (e.g. "eth", "arb1", "matic") to their EIP-155 chain IDs.
//
// Source: https://docs.safe.global/core-api/transaction-service-supported-networks
// and the EIP-3770 short-name registry. Keep in sync with safe/txservice.defaultURLs.
var SafeShortNameChainID = map[string]uint64{
	"eth":     1,
	"oeth":    10,
	"bnb":     56,
	"gno":     100,
	"matic":   137,
	"ftm":     250,
	"zkevm":   1101,
	"base":    8453,
	"arb1":    42161,
	"avax":    43114,
	"linea":   59144,
	"scr":     534352,
	"sep":     11155111,
	"basesep": 84532,
	"celo":    42220,
	"mantle":  5000,
	"blast":   81457,
	"sonic":   146,
	"aurora":  1313161554,
}

// SafeAppRef is what jarvis extracts from a Safe-app URL or an EIP-3770
// short-form reference. SafeTxHash is the zero value when the URL only
// identifies a Safe (queue / home pages, EIP-3770 references) and no
// specific pending transaction.
type SafeAppRef struct {
	ChainShortName string         // e.g. "eth", "arb1"; empty if not derivable
	ChainID        uint64         // resolved from ChainShortName, 0 if unknown
	SafeAddress    common.Address // never zero on a successful parse
	SafeTxHash     [32]byte       // zero if the URL didn't carry a tx hash
}

// HasTxHash returns true when the URL identifies a specific pending tx.
func (r *SafeAppRef) HasTxHash() bool {
	var zero [32]byte
	return r != nil && r.SafeTxHash != zero
}

// safeURLHosts is the set of hostnames Safe currently uses for its web UI.
// Other deployments (self-hosted, gnosis-safe.io legacy) follow the same
// query-string contract so we accept any host but still verify the path or
// query shape before claiming the input is a Safe URL.
var safeURLHosts = map[string]bool{
	"app.safe.global":     true,
	"safe.global":         true,
	"gnosis-safe.io":      true,
	"app.gnosis-safe.io":  true,
	"old.safe.global":     true,
	"holesky-safe.protofire.io": true, // popular community deployment
}

// idMultisigRe extracts safe + safeTxHash from the canonical Safe app
// transaction-detail query parameter:
//
//	id=multisig_<safeAddress>_<safeTxHash>
//
// Both addresses are matched case-insensitively (Safe URLs sometimes use
// EIP-55 checksummed addresses, sometimes plain lowercase).
var idMultisigRe = regexp.MustCompile(
	`(?i)multisig_(0x[0-9a-f]{40})_(0x[0-9a-f]{64})`,
)

// safeQueryRe matches the EIP-3770 short-form Safe references that Safe
// uses in its `?safe=` query parameter, e.g. `eth:0x71f8...`. It anchors
// to the whole input so a bare `eth:0x...` paste is recognised but a URL
// containing `safe=eth:0x...` mid-string is not (use chainPrefixRe for that).
var safeQueryRe = regexp.MustCompile(
	`(?i)^([a-z0-9]+):(0x[0-9a-f]{40})$`,
)

// chainPrefixRe finds an unanchored `<chainShortName>:<addr>` occurrence
// inside a larger string, used as a best-effort hint when we recover a
// Safe transaction id from a fragmented URL (case 2 of ParseSafeAppURL).
var chainPrefixRe = regexp.MustCompile(
	`(?i)\b([a-z]{2,10}[0-9]{0,3}):(0x[0-9a-f]{40})\b`,
)

// ParseSafeAppURL recognises four input flavours and returns the extracted
// reference:
//
//  1. A Safe-app web URL of the form
//     https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>&safe=<chain>:<safe>
//     (also matches /home, /transactions/queue, etc. — anything that carries
//     a `safe=` and/or `id=multisig_..._...` query parameter).
//  2. A bare Safe-app transaction id token, with or without the `id=` prefix
//     and with or without surrounding URL noise:
//     `multisig_<safe>_<hash>` or `id=multisig_<safe>_<hash>` or even a
//     truncated URL like `https://app.safe.global/transactions/tx?id=multisig_<safe>_<hash>`
//     (this last one is what a user typically ends up with when they paste
//     a Safe-app URL into a shell without quoting and the `&` gets lost).
//  3. An EIP-3770 short reference like `eth:0x71f8...`.
//  4. A bare 0x-prefixed Safe address — accepted as a no-op (chain unknown).
//
// On success, ok=true and the returned SafeAppRef has SafeAddress populated
// (and optionally ChainShortName/ChainID/SafeTxHash). Inputs that don't look
// like any of the above return ok=false with no error.
func ParseSafeAppURL(in string) (*SafeAppRef, bool) {
	in = strings.TrimSpace(in)
	if in == "" {
		return nil, false
	}

	// Case 4: bare address.
	if common.IsHexAddress(in) {
		return &SafeAppRef{SafeAddress: common.HexToAddress(in)}, true
	}

	// Case 3: EIP-3770 short form.
	if m := safeQueryRe.FindStringSubmatch(in); m != nil {
		ref := &SafeAppRef{
			ChainShortName: strings.ToLower(m[1]),
			SafeAddress:    common.HexToAddress(m[2]),
		}
		ref.ChainID = SafeShortNameChainID[ref.ChainShortName]
		return ref, true
	}

	// Case 2: a `multisig_<safe>_<hash>` token, possibly prefixed by `id=`
	// or wrapped in a URL fragment that lost its `?safe=...` half (e.g. an
	// unquoted shell paste). We don't require a recognised host here: the
	// `multisig_<addr>_<hash>` shape is unambiguous on its own.
	if m := idMultisigRe.FindStringSubmatch(in); m != nil {
		ref := &SafeAppRef{
			SafeAddress: common.HexToAddress(m[1]),
		}
		copy(ref.SafeTxHash[:], common.FromHex(m[2]))
		// Best-effort chain extraction in case a `safe=<chain>:<addr>`
		// fragment also survived (rare but cheap to support).
		if mm := chainPrefixRe.FindStringSubmatch(in); mm != nil {
			ref.ChainShortName = strings.ToLower(mm[1])
			ref.ChainID = SafeShortNameChainID[ref.ChainShortName]
		}
		return ref, true
	}

	// Case 1: full URL. Be permissive about scheme and host; the unique
	// signal is the presence of `safe=...` or `id=multisig_..._...`.
	u, err := url.Parse(in)
	if err != nil {
		return nil, false
	}
	q := u.Query()

	idVal := q.Get("id")
	safeVal := q.Get("safe")
	if idVal == "" && safeVal == "" {
		// Not a Safe URL we recognise. We require at least one of these
		// params so we don't accidentally claim arbitrary URLs.
		return nil, false
	}

	ref := &SafeAppRef{}

	if m := idMultisigRe.FindStringSubmatch(idVal); m != nil {
		ref.SafeAddress = common.HexToAddress(m[1])
		copy(ref.SafeTxHash[:], common.FromHex(m[2]))
	}

	if m := safeQueryRe.FindStringSubmatch(safeVal); m != nil {
		// Always trust the explicit `safe=<chain>:<addr>` for chain
		// resolution; for the address, only use it when id= didn't already
		// give us one. The two are virtually always identical.
		ref.ChainShortName = strings.ToLower(m[1])
		ref.ChainID = SafeShortNameChainID[ref.ChainShortName]
		var zero common.Address
		if ref.SafeAddress == zero {
			ref.SafeAddress = common.HexToAddress(m[2])
		}
	}

	var zero common.Address
	if ref.SafeAddress == zero {
		// We had a Safe-flavoured query string but no parseable address.
		// Treat this as a non-match so the caller can produce a clearer
		// downstream error ("not a contract address") instead of us
		// returning a half-baked reference.
		return nil, false
	}

	// Reject obviously non-safe hostnames if the input was actually a URL.
	// We do allow unknown hosts because Safe has many community deployments;
	// the host check just skips obviously-wrong inputs like
	// `https://etherscan.io/?id=multisig_0x..._0x...` from being accepted.
	if u.Host != "" && !isLikelySafeHost(u.Host) {
		return nil, false
	}

	return ref, true
}

func isLikelySafeHost(h string) bool {
	h = strings.ToLower(h)
	if safeURLHosts[h] {
		return true
	}
	for known := range safeURLHosts {
		if strings.HasSuffix(h, "."+known) || strings.HasSuffix(h, known) {
			return true
		}
	}
	// Self-hosted UIs commonly use a "safe." subdomain or a "-safe." infix.
	if strings.Contains(h, "safe") {
		return true
	}
	return false
}

// SafeTxHashHex returns the parsed safeTxHash as a 0x-prefixed lowercase
// hex string, or an empty string if no hash was present.
func (r *SafeAppRef) SafeTxHashHex() string {
	if !r.HasTxHash() {
		return ""
	}
	return fmt.Sprintf("0x%x", r.SafeTxHash[:])
}
