package kyber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	productionBase = "https://api.kyberswap.com/data/v1"
	userAgent      = "jarvis"
	envKey         = "JARVIS_KYBER_KEY"
	envURL         = "JARVIS_KYBER_URL"
)

// Client calls the Kyber Data API.
type Client struct {
	key  string
	base string
	http *http.Client
}

// NewClientFromEnv reads JARVIS_KYBER_KEY and the optional JARVIS_KYBER_URL.
func NewClientFromEnv() (*Client, error) {
	key := strings.TrimSpace(os.Getenv(envKey))
	if key == "" {
		return nil, fmt.Errorf("set %s to a Kyber Data API key (https://apigw-admin.kyberswap.com/)", envKey)
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(envURL)), "/")
	if base == "" {
		base = productionBase
	}
	return &Client{
		key:  key,
		base: base,
		http: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// Base returns the API origin the client is calling.
func (c *Client) Base() string { return c.base }

type apiError struct {
	Status int
	Path   string
	Body   string
}

func (e *apiError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 300 {
		body = body[:300]
	}
	if body == "" {
		return fmt.Sprintf("kyber data api %s: http %d", e.Path, e.Status)
	}
	return fmt.Sprintf("kyber data api %s: http %d: %s", e.Path, e.Status, body)
}

func (c *Client) get(ctx context.Context, path string, q url.Values, dest any) error {
	_, err := c.doExchange(ctx, http.MethodGet, path, q, nil, dest)
	return err
}

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	_, err := c.doExchange(ctx, http.MethodPost, path, nil, body, dest)
	return err
}

// Exchange is one call as it was sent, plus the response body. The API key
// stays in the header and is not part of URL.
type Exchange struct {
	Method string
	URL    string
	Raw    string
}

func (c *Client) doExchange(ctx context.Context, method, path string, q url.Values, body any, dest any) (Exchange, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return Exchange{}, err
		}
	}
	ex := Exchange{Method: method, URL: c.base + path}
	if len(q) > 0 {
		ex.URL += "?" + q.Encode()
	}
	for attempt := 0; attempt < 2; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, ex.URL, rdr)
		if err != nil {
			return ex, err
		}
		req.Header.Set("x-api-key", c.key)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return ex, fmt.Errorf("kyber data api %s: %w", path, err)
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		ex.Raw = string(raw)
		if err != nil {
			return ex, fmt.Errorf("kyber data api %s: %w", path, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			wait := retryAfter(resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return ex, ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return ex, &apiError{Status: resp.StatusCode, Path: path, Body: string(raw)}
		}
		var env struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return ex, fmt.Errorf("kyber data api %s: %w", path, err)
		}
		if env.Code != 0 {
			if env.Message == "" {
				env.Message = "request failed"
			}
			return ex, fmt.Errorf("kyber data api %s: %s", path, env.Message)
		}
		if dest == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return ex, nil
		}
		if err := json.Unmarshal(env.Data, dest); err != nil {
			return ex, fmt.Errorf("kyber data api %s: %w", path, err)
		}
		return ex, nil
	}
	return ex, fmt.Errorf("kyber data api %s: rate limited", path)
}

func retryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || secs < 1 {
		return time.Second
	}
	if secs > 15 {
		secs = 15
	}
	return time.Duration(secs) * time.Second
}
