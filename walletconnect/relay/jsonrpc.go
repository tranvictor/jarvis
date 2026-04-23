package relay

import (
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 envelope, specialised to the irn_ methods we care about.
//
// We don't use a full jsonrpc library because:
//   - we only speak five methods (subscribe, publish, unsubscribe,
//     subscription, batchSubscribe) and only ever see responses for
//     requests we ourselves initiated;
//   - the relay's "result" field is polymorphic (string for
//     subscribe, bool for publish, object for subscription-ack) and a
//     generic library would either force json.RawMessage everywhere
//     anyway or hide that polymorphism behind reflection.

type rpcRequest struct {
	ID      uint64          `json:"id"`
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	ID      uint64          `json:"id"`
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	if e.Data != "" {
		return fmt.Sprintf("relay rpc error %d: %s (%s)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("relay rpc error %d: %s", e.Code, e.Message)
}

// subscribeParams is the body of irn_subscribe.
type subscribeParams struct {
	Topic string `json:"topic"`
}

// publishParams is the body of irn_publish. The relay requires tag +
// ttl — they are what it uses to route push-notifications and enforce
// retention. Prompt is a hint to the peer that this is a user-facing
// request (shown in some wallets' lock-screen previews); we set it on
// wc_sessionRequest only.
type publishParams struct {
	Topic   string `json:"topic"`
	Message string `json:"message"`
	TTL     uint64 `json:"ttl"`
	Tag     uint64 `json:"tag"`
	Prompt  bool   `json:"prompt,omitempty"`
}

// subscriptionParams is the body of the server-initiated
// irn_subscription notification.
type subscriptionParams struct {
	ID   string `json:"id"`
	Data struct {
		Topic       string `json:"topic"`
		Message     string `json:"message"`
		PublishedAt int64  `json:"publishedAt,omitempty"`
		Tag         uint64 `json:"tag,omitempty"`
	} `json:"data"`
}
