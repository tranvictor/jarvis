// Package shim serves the Esplora HTTP API on top of a bitcoind node, so the
// end-to-end tests can run Jarvis against a real consensus engine: every
// transaction Jarvis broadcasts is validated by bitcoind itself.
package shim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RPC struct {
	URL, User, Pass string
	HTTP            *http.Client
}

func NewRPC(url, user, pass string) *RPC {
	return &RPC{URL: url, User: user, Pass: pass, HTTP: &http.Client{Timeout: 60 * time.Second}}
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

func (r *RPC) Call(method string, params ...any) (json.RawMessage, error) {
	if params == nil {
		params = []any{}
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": "e2e", "method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, r.URL, bytes.NewReader(body))
	req.SetBasicAuth(r.User, r.Pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("bitcoind %s: HTTP %d: %s", method, resp.StatusCode, string(data))
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Result, nil
}

func (r *RPC) CallInto(out any, method string, params ...any) error {
	raw, err := r.Call(method, params...)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
