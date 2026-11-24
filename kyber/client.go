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
	return c.do(ctx, http.MethodGet, path, q, nil, dest)
}

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, dest)
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, dest any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for attempt := 0; attempt < 2; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rdr)
		if err != nil {
			return err
		}
		req.Header.Set("x-api-key", c.key)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("kyber data api %s: %w", path, err)
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("kyber data api %s: %w", path, err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			wait := retryAfter(resp.Header.Get("Retry-After"))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &apiError{Status: resp.StatusCode, Path: path, Body: string(raw)}
		}
		var env struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("kyber data api %s: %w", path, err)
		}
		if env.Code != 0 {
			if env.Message == "" {
				env.Message = "request failed"
			}
			return fmt.Errorf("kyber data api %s: %s", path, env.Message)
		}
		if dest == nil || len(env.Data) == 0 || string(env.Data) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Data, dest); err != nil {
			return fmt.Errorf("kyber data api %s: %w", path, err)
		}
		return nil
	}
	return fmt.Errorf("kyber data api %s: rate limited", path)
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
