package txservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a thin HTTP client for the Safe Transaction Service REST API.
//
// The client is intentionally small: it covers exactly the endpoints jarvis
// uses (`init`, `approve`, `execute`) and surfaces server-side validation
// errors (400-class responses) verbatim so the user can see what went wrong.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a Client targetting the canonical Safe Transaction
// Service for the given chain ID, honoring SAFE_TX_SERVICE_URL[_<chainID>]
// environment overrides.
func NewClient(chainID uint64) (*Client, error) {
	base, err := URLForChain(chainID)
	if err != nil {
		return nil, err
	}
	return &Client{
		BaseURL: base,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) doJSON(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Surface the server message verbatim — Safe service returns
		// JSON validation errors that are very useful to the user.
		msg := strings.TrimSpace(string(respBody))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, msg)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decode %s %s response: %w (body=%s)", method, path, err, string(respBody))
	}
	return nil
}

// GetSafe returns the SafeInfo for safe (owners, threshold, current nonce,
// version). Returns an error if the Safe is not yet indexed by the service.
func (c *Client) GetSafe(safe string) (*SafeInfo, error) {
	out := &SafeInfo{}
	if err := c.doJSON(http.MethodGet,
		fmt.Sprintf("/api/v1/safes/%s/", checksumPath(safe)),
		nil, out,
	); err != nil {
		return nil, err
	}
	return out, nil
}

// GetTx fetches a multisig transaction by its safeTxHash, including all
// confirmations collected so far.
func (c *Client) GetTx(safeTxHash string) (*MultisigTx, error) {
	out := &MultisigTx{}
	if err := c.doJSON(http.MethodGet,
		fmt.Sprintf("/api/v1/multisig-transactions/%s/", strings.ToLower(safeTxHash)),
		nil, out,
	); err != nil {
		return nil, err
	}
	return out, nil
}

// ListPending returns multisig transactions for safe whose nonce is >= the
// Safe's current on-chain nonce (i.e. not yet executed). Optional filters
// can be provided in q (e.g. "nonce=5").
func (c *Client) ListPending(safe string, q url.Values) ([]MultisigTx, error) {
	out := struct {
		Results []MultisigTx `json:"results"`
	}{}
	path := fmt.Sprintf("/api/v1/safes/%s/multisig-transactions/", checksumPath(safe))
	if q == nil {
		q = url.Values{}
	}
	q.Set("executed", "false")
	if _, ok := q["ordering"]; !ok {
		q.Set("ordering", "nonce")
	}
	path += "?" + q.Encode()
	if err := c.doJSON(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

// FindByNonce returns the (single) multisig transaction queued for safe at
// the given nonce, or nil if none exists. Multiple proposals can technically
// share a nonce; this returns the first match.
func (c *Client) FindByNonce(safe string, nonce uint64) (*MultisigTx, error) {
	q := url.Values{}
	q.Set("nonce", strconv.FormatUint(nonce, 10))
	txs, err := c.ListPending(safe, q)
	if err != nil {
		return nil, err
	}
	if len(txs) == 0 {
		return nil, nil
	}
	return &txs[0], nil
}

// Propose submits a new multisig transaction to the service together with
// the proposing owner's signature (typically gathered via Account.SignSafeHash).
func (c *Client) Propose(safe string, req *ProposeTxRequest) error {
	return c.doJSON(http.MethodPost,
		fmt.Sprintf("/api/v1/safes/%s/multisig-transactions/", checksumPath(safe)),
		req, nil,
	)
}

// Confirm appends a single owner's signature to an existing pending tx.
func (c *Client) Confirm(safeTxHash string, req *ConfirmRequest) error {
	return c.doJSON(http.MethodPost,
		fmt.Sprintf("/api/v1/multisig-transactions/%s/confirmations/", strings.ToLower(safeTxHash)),
		req, nil,
	)
}

// checksumPath returns addr unchanged. The Safe service is permissive about
// the checksum case in URL path components but normalises the response,
// so we deliberately do not re-checksum here to avoid pulling go-ethereum
// into this package solely for that.
func checksumPath(addr string) string {
	return addr
}
