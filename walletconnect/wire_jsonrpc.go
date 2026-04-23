package walletconnect

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// jsonrpcError is the wire form of JSON-RPC 2.0 errors (session + pairing).
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// idUint64FromRaw decodes a JSON-RPC id from a raw JSON value (string or
// number) without going through float64, so large ids and string ids
// from browser JSON.stringify both work.
func idUint64FromRaw(raw json.RawMessage) (uint64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("empty id")
	}
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return 0, err
		}
		return strconv.ParseUint(str, 10, 64)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return strconv.ParseUint(n.String(), 10, 64)
}

// wireRPC is a decoded JSON-RPC frame from a peer (dApp) after WC decrypt.
// ID is always json.RawMessage so we never depend on uint64 struct tags
// (which break on string "id" values).
type wireRPC struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonrpcError   `json:"error"`
}

func decodeWireRPC(plaintext []byte) (*wireRPC, error) {
	var w wireRPC
	if err := json.Unmarshal(plaintext, &w); err != nil {
		return nil, err
	}
	return &w, nil
}
