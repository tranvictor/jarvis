package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultURL   = "https://api.x.ai/v1/chat/completions"
	DefaultModel = "grok-4.6"
	DefaultWait  = 45 * time.Second
)

// Client talks to xAI Chat Completions. It accepts only a marshaled Payload.
type Client struct {
	HTTP  *http.Client
	URL   string
	Model string
	Key   string
}

// NewFromEnv builds a Client from XAI_API_KEY. Key empty means the caller
// should skip the AI measure rather than send.
func NewFromEnv() *Client {
	key := os.Getenv("XAI_API_KEY")
	return &Client{
		HTTP:  &http.Client{Timeout: DefaultWait},
		URL:   DefaultURL,
		Model: DefaultModel,
		Key:   key,
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Reply is the parsed model JSON.
type Reply struct {
	Risk        string   `json:"risk"`
	Bullets     []string `json:"bullets"`
	AssetEffect string   `json:"asset_effect"`
	Reconfirms  []string `json:"reconfirms"`
}

// Complete posts payloadJSON as the user message. payloadJSON must be
// json.Marshal of a Payload — the caller builds that in package vet.
func (c *Client) Complete(ctx context.Context, payloadJSON []byte) (Reply, error) {
	if c == nil {
		return Reply{}, fmt.Errorf("ai: nil client")
	}
	if c.Key == "" {
		return Reply{}, fmt.Errorf("ai: XAI_API_KEY is not set")
	}
	url := c.URL
	if url == "" {
		url = DefaultURL
	}
	model := c.Model
	if model == "" {
		model = DefaultModel
	}
	body, err := json.Marshal(chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: string(payloadJSON)},
		},
	})
	if err != nil {
		return Reply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultWait}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Reply{}, err
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Reply{}, fmt.Errorf("ai: decode response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return Reply{}, fmt.Errorf("ai: %s", parsed.Error.Message)
	}
	if resp.StatusCode >= 400 {
		return Reply{}, fmt.Errorf("ai: HTTP %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return Reply{}, fmt.Errorf("ai: empty choices")
	}
	return parseReply(parsed.Choices[0].Message.Content)
}

func parseReply(content string) (Reply, error) {
	content = stripJSONFence(content)
	var r Reply
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		return Reply{}, fmt.Errorf("ai: model JSON: %w", err)
	}
	return r, nil
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimPrefix(s, "JSON")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}
