package relay

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Delivery is an incoming message routed by the relay to a subscribed
// topic. Message is still base64-encoded — the relay is agnostic to
// WC envelope contents.
type Delivery struct {
	Topic       string
	Message     string
	PublishedAt int64
	// Tag is the irn_publish tag (1100=…, 1102=session settle, …); 0 if omitted.
	Tag uint64
}

// Config collects the knobs Dial needs to bring up a connection.
type Config struct {
	// URL of the relay, e.g. "wss://relay.walletconnect.org".
	URL string
	// ProjectID registered at cloud.reown.com. Required; the relay
	// rejects unauthenticated connections with 401.
	ProjectID string
	// AuthJWT signed by BuildRelayJWT. Also required.
	AuthJWT string
	// DialTimeout bounds the initial connect. Defaults to 15s.
	DialTimeout time.Duration
	// WriteTimeout bounds each outgoing frame. Defaults to 10s.
	WriteTimeout time.Duration
	// ReadTimeout bounds the overall read deadline; the client
	// extends it on every pong. Defaults to 60s.
	ReadTimeout time.Duration
}

// Client is a single live connection to the WC relay. One per session.
//
// Writes are serialised through a channel so the reader goroutine and
// ad-hoc publish/subscribe calls don't race on the underlying
// websocket. Pending RPC calls are tracked in a map keyed by id so
// the read loop can match responses back to the caller.
type Client struct {
	cfg       Config
	conn      *websocket.Conn
	writeCh   chan []byte
	incoming  chan Delivery
	done      chan struct{}
	closeOnce sync.Once

	mu      sync.Mutex
	pending map[uint64]chan rpcResponse
	closed  bool
	err     error
}

// Dial opens a WebSocket to cfg.URL with the WC-required query params
// and starts the read+write goroutines. Returns a client ready to
// Subscribe / Publish.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("relay: URL is required")
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("relay: ProjectID is required (register one at cloud.reown.com)")
	}
	if cfg.AuthJWT == "" {
		return nil, fmt.Errorf("relay: AuthJWT is required")
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 15 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 60 * time.Second
	}

	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse relay url: %w", err)
	}
	q := u.Query()
	q.Set("projectId", cfg.ProjectID)
	q.Set("auth", cfg.AuthJWT)
	u.RawQuery = q.Encode()

	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = cfg.DialTimeout

	dctx, cancel := context.WithTimeout(ctx, cfg.DialTimeout)
	defer cancel()

	conn, resp, err := dialer.DialContext(dctx, u.String(), nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("relay dial %s: %w (status %s)", cfg.URL, err, resp.Status)
		}
		return nil, fmt.Errorf("relay dial %s: %w", cfg.URL, err)
	}

	c := &Client{
		cfg:      cfg,
		conn:     conn,
		writeCh:  make(chan []byte, 16),
		incoming: make(chan Delivery, 16),
		done:     make(chan struct{}),
		pending:  map[uint64]chan rpcResponse{},
	}

	// Ping/pong keepalive. Relay tolerates ~30s idle; we ping every
	// 20s and extend the read deadline on pong.
	conn.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(cfg.ReadTimeout))
		return nil
	})

	go c.writeLoop()
	go c.readLoop()
	go c.pingLoop()

	return c, nil
}

// Incoming returns the channel of server-pushed irn_subscription
// notifications. The channel is closed when the connection drops.
func (c *Client) Incoming() <-chan Delivery { return c.incoming }

// Done reports when the client has shut down. Err() then returns the
// terminating error (nil for a user-initiated close).
func (c *Client) Done() <-chan struct{} { return c.done }

// Err returns the error that terminated the connection, if any.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close shuts down the client. Safe to call multiple times.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		_ = c.conn.Close()
		close(c.done)
		c.mu.Lock()
		c.closed = true
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	})
	return nil
}

// Subscribe registers this client for topic and returns the
// subscription id allocated by the relay. Incoming deliveries for
// the topic arrive via Incoming().
func (c *Client) Subscribe(ctx context.Context, topic string) (string, error) {
	params, _ := json.Marshal(subscribeParams{Topic: topic})
	raw, err := c.call(ctx, "irn_subscribe", params)
	if err != nil {
		return "", err
	}
	var subID string
	if err := json.Unmarshal(raw, &subID); err != nil {
		return "", fmt.Errorf("subscribe result is not a string: %w (raw=%s)", err, string(raw))
	}
	return subID, nil
}

// Publish sends message (already base64-encoded WC envelope) to topic
// with the WC-prescribed (tag, ttl) pair. prompt=true hints to push
// infrastructure that this is a foreground request.
func (c *Client) Publish(
	ctx context.Context,
	topic, message string,
	tag uint64, ttl time.Duration, prompt bool,
) error {
	params, _ := json.Marshal(publishParams{
		Topic:   topic,
		Message: message,
		TTL:     uint64(ttl.Seconds()),
		Tag:     tag,
		Prompt:  prompt,
	})
	raw, err := c.call(ctx, "irn_publish", params)
	if err != nil {
		return err
	}
	var ok bool
	if err := json.Unmarshal(raw, &ok); err != nil {
		// Some relay builds return `null` instead of `true`. Treat
		// the absence of an error as success.
		return nil
	}
	if !ok {
		return fmt.Errorf("relay responded false to irn_publish on %s", topic)
	}
	return nil
}

// call serialises a JSON-RPC request, registers a response channel,
// writes the frame, and blocks for the response.
func (c *Client) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := newPayloadID()
	req := rpcRequest{ID: id, JSONRPC: "2.0", Method: method, Params: params}
	frame, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", method, err)
	}

	ch := make(chan rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("relay closed: %w", c.err)
	}
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	select {
	case c.writeCh <- frame:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("relay closed while sending %s: %w", method, c.err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("relay closed before %s responded", method)
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("relay closed while awaiting %s: %w", method, c.err)
	}
}

// writeLoop owns the socket's write side. Serialising here avoids
// interleaving RPC frames with pings/acks.
func (c *Client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case msg, ok := <-c.writeCh:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.fail(fmt.Errorf("write: %w", err))
				return
			}
		}
	}
}

// pingLoop keeps the connection alive. The relay will close idle
// connections after ~30s.
func (c *Client) pingLoop() {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				c.fail(fmt.Errorf("ping: %w", err))
				return
			}
		}
	}
}

// readLoop dispatches incoming frames to the right handler:
//   - responses go to the caller's channel via pending[id]
//   - irn_subscription requests are re-emitted on c.incoming and
//     acked back to the relay
//
// Any read error tears the connection down.
func (c *Client) readLoop() {
	defer close(c.incoming)
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.fail(fmt.Errorf("read: %w", err))
			return
		}

		// Peek at whether this is a request or response. The relay may
		// send "id" as a JSON string or number; never decode into uint64.
		var frame struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *rpcError       `json:"error"`
		}
		if err := json.Unmarshal(data, &frame); err != nil {
			c.fail(fmt.Errorf("decode relay frame: %w (raw=%s)", err, string(data)))
			return
		}

		if frame.Method == "" {
			// Response to one of our irn_* calls.
			idU64, err := uint64FromJSONID(frame.ID)
			if err != nil {
				c.fail(fmt.Errorf("decode relay response id: %w (raw=%s)", err, string(data)))
				return
			}
			c.mu.Lock()
			ch, ok := c.pending[idU64]
			c.mu.Unlock()
			if ok {
				ch <- rpcResponse{ID: idU64, Result: frame.Result, Error: frame.Error}
			}
			continue
		}

		// Request — we only know irn_subscription. Echo the same
		// JSON "id" (string or number) the relay sent, or the server
		// will keep retrying the notification and our subscription
		// stream starves.
		ack, _ := json.Marshal(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  bool            `json:"result"`
		}{JSONRPC: "2.0", ID: frame.ID, Result: true})

		switch frame.Method {
		case "irn_subscription":
			var sp subscriptionParams
			if err := json.Unmarshal(frame.Params, &sp); err != nil {
				c.fail(fmt.Errorf("decode subscription params: %w", err))
				return
			}
			select {
			case c.writeCh <- ack:
			case <-c.done:
				return
			}
			select {
			case c.incoming <- Delivery{
				Topic:       sp.Data.Topic,
				Message:     sp.Data.Message,
				PublishedAt: sp.Data.PublishedAt,
				Tag:         sp.Data.Tag,
			}:
			case <-c.done:
				return
			}
		default:
			// Unknown server-initiated method: ack to keep the relay
			// happy, otherwise ignore.
			select {
			case c.writeCh <- ack:
			case <-c.done:
				return
			}
		}
	}
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
	_ = c.Close()
}

// newPayloadID returns a JSON-RPC id in the WalletConnect-sanctioned
// shape: `Date.now() (ms) * 10^6 + random_6_digits`. The relay
// validates the id as an int64 microsecond-precision timestamp; a
// monotonically-incrementing small integer is rejected with
// -32600 "Invalid request ID". This matches the behaviour of
// @walletconnect/jsonrpc-utils payloadId(entropy=6).
//
// The result comfortably fits in a uint64: millis is ~1.7e12, so
// millis*1e6 is ~1.7e18, under math.MaxUint64 (~1.8e19).
func newPayloadID() uint64 {
	ms := uint64(time.Now().UnixMilli())
	var b [8]byte
	// crypto/rand never errors in practice on supported platforms;
	// fall back to the low 20 bits of the nanosecond clock if it
	// ever does.
	if _, err := rand.Read(b[:]); err != nil {
		return ms*1_000_000 + uint64(time.Now().UnixNano())&0xFFFFF
	}
	return ms*1_000_000 + binary.BigEndian.Uint64(b[:])%1_000_000
}
