package walletconnect

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// URI is a parsed WalletConnect pairing URI as defined by WCIP-5
// (https://specs.walletconnect.com/2.0/specs/clients/core/pairing/pairing-uri).
//
// v2 shape:
//
//	wc:<topic>@2?relay-protocol=<name>&symKey=<32-byte-hex>[&relay-data=<json>][&expiryTimestamp=<unix>]
//
// Only the four starred fields matter for pairing:
//
//	Topic         - 32-byte hex pairing topic (no 0x prefix)
//	RelayProtocol - almost always "irn"; we still parse it generically
//	SymKey        - 32-byte hex symmetric key used to decrypt the first
//	                session_propose envelope before Diffie-Hellman kicks in
//	ExpiryTS      - optional unix seconds; advisory, we don't enforce it
//	                at parse time (the relay/session layer decides).
//
// RelayData is opaque JSON some dApps include; we preserve it verbatim
// so the session layer can pass it through when subscribing if the
// relay eventually requires it.
type URI struct {
	Topic         string
	Version       int
	RelayProtocol string
	RelayData     string
	SymKey        []byte
	ExpiryTS      int64
}

// ParseURI parses a wc: URI. It intentionally does only syntactic
// validation — the returned URI will always be usable for pairing, but
// ParseURI does not try to pre-validate signatures or contact the relay.
//
// Errors from this function are safe to surface to end users verbatim.
func ParseURI(raw string) (*URI, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty WalletConnect URI")
	}
	if !strings.HasPrefix(raw, "wc:") {
		return nil, fmt.Errorf("WalletConnect URI must start with 'wc:' (got %q)", truncateForError(raw))
	}

	// The url package can't parse "wc:<topic>@<ver>?..." directly — the
	// '@' confuses it into thinking topic is a userinfo section. Split
	// by hand: everything between "wc:" and "?" is the path-and-version
	// component, everything after "?" is the query string.
	rest := strings.TrimPrefix(raw, "wc:")
	head, query := rest, ""
	if i := strings.Index(rest, "?"); i >= 0 {
		head = rest[:i]
		query = rest[i+1:]
	}

	// head := "<topic>@<version>"
	at := strings.LastIndex(head, "@")
	if at < 0 {
		return nil, fmt.Errorf("WalletConnect URI missing '@<version>': %q", truncateForError(raw))
	}
	topic := strings.ToLower(strings.TrimSpace(head[:at]))
	verStr := strings.TrimSpace(head[at+1:])
	if topic == "" {
		return nil, fmt.Errorf("WalletConnect URI missing topic before '@'")
	}
	if !isHex(topic) || len(topic) != 64 {
		return nil, fmt.Errorf("WalletConnect URI topic must be 32-byte hex, got %d chars", len(topic))
	}
	version, err := strconv.Atoi(verStr)
	if err != nil || version <= 0 {
		return nil, fmt.Errorf("WalletConnect URI has invalid version %q", verStr)
	}
	if version == 1 {
		return nil, fmt.Errorf(
			"this is a WalletConnect v1 URI; jarvis only supports v2. " +
				"Ask the dApp for a WalletConnect v2 connection (most dApps show both QRs)")
	}
	if version != 2 {
		return nil, fmt.Errorf(
			"unsupported WalletConnect URI version %d; jarvis only supports v2", version)
	}

	vals, err := url.ParseQuery(query)
	if err != nil {
		return nil, fmt.Errorf("WalletConnect URI query is malformed: %w", err)
	}

	relayProto := strings.TrimSpace(vals.Get("relay-protocol"))
	if relayProto == "" {
		// Default to irn per spec when the field is omitted. A handful
		// of implementations skip it entirely; being permissive here
		// costs nothing.
		relayProto = "irn"
	}
	symKeyHex := strings.TrimSpace(vals.Get("symKey"))
	if symKeyHex == "" {
		return nil, fmt.Errorf("WalletConnect URI missing required symKey parameter")
	}
	symKey, err := hex.DecodeString(strings.TrimPrefix(symKeyHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("WalletConnect URI symKey is not valid hex: %w", err)
	}
	if len(symKey) != 32 {
		return nil, fmt.Errorf(
			"WalletConnect URI symKey must decode to 32 bytes, got %d", len(symKey))
	}

	var expiry int64
	if s := strings.TrimSpace(vals.Get("expiryTimestamp")); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			expiry = n
		}
	}

	return &URI{
		Topic:         topic,
		Version:       version,
		RelayProtocol: relayProto,
		RelayData:     vals.Get("relay-data"),
		SymKey:        symKey,
		ExpiryTS:      expiry,
	}, nil
}

// isHex reports whether s is a non-empty string of only hex digits. We
// deliberately accept both cases (spec says lowercase but dApps do vary).
func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// truncateForError shortens a URI for inclusion in an error message —
// a full wc: URI can run to 200+ characters and drowning the user in it
// for a parse error is unhelpful.
func truncateForError(s string) string {
	const max = 80
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
