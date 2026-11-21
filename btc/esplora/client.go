// Package esplora is a small client for the Esplora HTTP API served by
// mempool.space, blockstream.info and self-hosted electrs/esplora nodes.
package esplora

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxBody = 8 << 20

type Client struct {
	Base string
	HTTP *http.Client
	// Retries is how many extra attempts a GET gets after a transport error,
	// a 429 or a 5xx. POST is never retried: a broadcast that timed out may
	// still have reached the network.
	Retries int
}

func New(base string) *Client {
	return &Client{
		Base:    strings.TrimRight(base, "/"),
		HTTP:    &http.Client{Timeout: 20 * time.Second},
		Retries: 2,
	}
}

type HTTPError struct {
	Method string
	URL    string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("esplora %s %s: HTTP %d: %s", e.Method, e.URL, e.Status, strings.TrimSpace(e.Body))
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if c.Base == "" {
		return nil, errors.New("no Esplora URL configured: pass --esplora or set JARVIS_BTC_ESPLORA")
	}
	attempts := 1
	if method == http.MethodGet {
		attempts += c.Retries
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-time.After(time.Duration(i) * 400 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.Base+path, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "jarvis-btc")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("esplora %s %s: %w", method, path, err)
			continue
		}
		data, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		resp.Body.Close()
		if rerr != nil {
			lastErr = fmt.Errorf("esplora %s %s: reading body: %w", method, path, rerr)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return data, nil
		}
		lastErr = &HTTPError{Method: method, URL: c.Base + path, Status: resp.StatusCode, Body: string(data)}
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("esplora GET %s: bad JSON: %w", path, err)
	}
	return nil
}

type stats struct {
	FundedTxoSum int64 `json:"funded_txo_sum"`
	SpentTxoSum  int64 `json:"spent_txo_sum"`
	TxCount      int   `json:"tx_count"`
}

type AddressStats struct {
	// Confirmed is funded minus spent over confirmed history, in satoshis.
	Confirmed int64
	// Pending is the net effect of unconfirmed transactions; it is negative
	// while an outgoing payment waits in the mempool.
	Pending int64
	// TxCount counts confirmed and unconfirmed transactions touching the
	// address. Zero means the address has never been used.
	TxCount int
}

func (c *Client) Address(ctx context.Context, addr string) (AddressStats, error) {
	var r struct {
		Chain   stats `json:"chain_stats"`
		Mempool stats `json:"mempool_stats"`
	}
	if err := c.getJSON(ctx, "/address/"+addr, &r); err != nil {
		return AddressStats{}, err
	}
	return AddressStats{
		Confirmed: r.Chain.FundedTxoSum - r.Chain.SpentTxoSum,
		Pending:   r.Mempool.FundedTxoSum - r.Mempool.SpentTxoSum,
		TxCount:   r.Chain.TxCount + r.Mempool.TxCount,
	}, nil
}

type UTXO struct {
	TxID      string `json:"txid"`
	Vout      uint32 `json:"vout"`
	Value     int64  `json:"value"`
	Confirmed bool   `json:"-"`
	Height    int64  `json:"-"`
}

func (c *Client) UTXOs(ctx context.Context, addr string) ([]UTXO, error) {
	var raw []struct {
		TxID   string `json:"txid"`
		Vout   uint32 `json:"vout"`
		Value  int64  `json:"value"`
		Status struct {
			Confirmed   bool  `json:"confirmed"`
			BlockHeight int64 `json:"block_height"`
		} `json:"status"`
	}
	if err := c.getJSON(ctx, "/address/"+addr+"/utxo", &raw); err != nil {
		return nil, err
	}
	out := make([]UTXO, 0, len(raw))
	for _, r := range raw {
		if len(r.TxID) != 64 || r.Value <= 0 {
			return nil, fmt.Errorf("esplora returned a malformed utxo for %s: %+v", addr, r)
		}
		out = append(out, UTXO{TxID: r.TxID, Vout: r.Vout, Value: r.Value, Confirmed: r.Status.Confirmed, Height: r.Status.BlockHeight})
	}
	return out, nil
}

// FeeEstimates maps a confirmation target in blocks to a rate in sat/vB.
func (c *Client) FeeEstimates(ctx context.Context) (map[int]float64, error) {
	var raw map[string]float64
	if err := c.getJSON(ctx, "/fee-estimates", &raw); err != nil {
		return nil, err
	}
	out := make(map[int]float64, len(raw))
	for k, v := range raw {
		n, err := strconv.Atoi(k)
		if err != nil || n <= 0 || v <= 0 {
			continue
		}
		out[n] = v
	}
	return out, nil
}

var ErrNoFeeEstimate = errors.New("the node has no fee estimate (regtest, or a node that is still syncing); pass --fee-rate")

// RateFor picks the estimate for the smallest published target that is at
// least the requested one, falling back to the closest lower target.
func RateFor(est map[int]float64, target int) (float64, int, error) {
	if len(est) == 0 {
		return 0, 0, ErrNoFeeEstimate
	}
	keys := make([]int, 0, len(est))
	for k := range est {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		if k >= target {
			return est[k], k, nil
		}
	}
	k := keys[len(keys)-1]
	return est[k], k, nil
}

func (c *Client) TipHeight(ctx context.Context) (int64, error) {
	data, err := c.do(ctx, http.MethodGet, "/blocks/tip/height", nil)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
}

// TxHex returns the raw serialized transaction.
func (c *Client) TxHex(ctx context.Context, txid string) (string, error) {
	data, err := c.do(ctx, http.MethodGet, "/tx/"+txid+"/hex", nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

type TxStatus struct {
	Confirmed   bool  `json:"confirmed"`
	BlockHeight int64 `json:"block_height"`
}

func (c *Client) TxStatus(ctx context.Context, txid string) (TxStatus, error) {
	var s TxStatus
	err := c.getJSON(ctx, "/tx/"+txid+"/status", &s)
	return s, err
}

// Broadcast submits a signed transaction and returns the txid the server
// reports. The caller must compare it with the txid it computed locally.
func (c *Client) Broadcast(ctx context.Context, rawHex string) (string, error) {
	data, err := c.do(ctx, http.MethodPost, "/tx", []byte(rawHex))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
