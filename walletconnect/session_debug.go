package walletconnect

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tranvictor/jarvis/walletconnect/relay"
)

// dbg logs a [wc:debug] line when verbose mode is on (set via --verbose or
// JARVIS_WC_VERBOSE=1 in jarvis wc).
func (s *Session) dbg(format string, args ...interface{}) {
	if s == nil || !s.verbose {
		return
	}
	s.ui.Info("[wc:debug] "+format, args...)
}

func truncStr(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// summaryWire returns a one-line description of a decoded wire frame for logs.
func summaryWire(w *wireRPC) string {
	if w == nil {
		return "nil"
	}
	if w.Method != "" {
		return fmt.Sprintf("method=%q id=%s paramsLen=%d", w.Method, string(truncStr(string(w.ID), 64)), len(w.Params))
	}
	if w.Error != nil {
		return fmt.Sprintf("response id=%s err=%d %s", string(truncStr(string(w.ID), 64)), w.Error.Code, truncStr(w.Error.Message, 120))
	}
	return fmt.Sprintf("response id=%s result=%s", string(truncStr(string(w.ID), 64)), truncStr(string(w.Result), 80))
}

// shortTopic returns the first 8 chars of a topic (or the whole thing
// if shorter) — enough for humans to tell pairing from session in
// non-verbose output.
func shortTopic(t string) string {
	if len(t) <= 10 {
		return t
	}
	return t[:10]
}

func redactKey32(label string, b [32]byte) string {
	hexb := fmt.Sprintf("%x", b[:])
	if len(hexb) < 12 {
		return label + hexb
	}
	return label + hexb[:8] + "…" + hexb[len(hexb)-4:]
}

func mustJSONSize(v interface{}) int {
	b, err := json.Marshal(v)
	if err != nil {
		return -1
	}
	return len(b)
}

// logDelivery logs relay irn_subscription metadata (ciphertext, not plain JSON).
func (s *Session) logDelivery(where string, del relay.Delivery) {
	if s == nil || !s.verbose {
		return
	}
	expect := "?"
	if strings.EqualFold(del.Topic, s.pairingTopic) {
		expect = "pairing"
	}
	if s.sessionTopic != "" && strings.EqualFold(del.Topic, s.sessionTopic) {
		expect = "session"
	}
	s.dbg("%s: topic=%s expect=%s tag=%d b64len=%d publishedAt=%d", where, del.Topic, expect, del.Tag, len(del.Message), del.PublishedAt)
}
