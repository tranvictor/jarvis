package kyber

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// MarketSocket is one live price connection. A read past 45 seconds of
// silence fails the connection; the caller reconnects and subscribes again.
type MarketSocket struct {
	url    string
	conn   *websocket.Conn
	write  sync.Mutex
	next   int
	Events chan MarketEvent

	mu      sync.Mutex
	waiters map[int]chan socketReply
	closed  chan struct{}
	once    sync.Once
}

type socketReply struct {
	code int
	msg  string
}

// OpenMarketSocket dials GET /tokens/market/socket.
func (c *Client) OpenMarketSocket(ctx context.Context) (*MarketSocket, error) {
	u := c.base + "/tokens/market/socket"
	u = strings.Replace(u, "https://", "wss://", 1)
	u = strings.Replace(u, "http://", "ws://", 1)
	h := http.Header{}
	h.Set("x-api-key", c.key)
	h.Set("User-Agent", userAgent)
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	conn, resp, err := dialer.DialContext(ctx, u, h)
	if err != nil {
		debugAppend("GET "+u, []byte("error: "+err.Error()))
		if resp != nil {
			return nil, fmt.Errorf("kyber market socket: %w (http %s)", err, resp.Status)
		}
		return nil, fmt.Errorf("kyber market socket: %w", err)
	}
	debugAppend("GET "+u, nil)
	s := &MarketSocket{
		url:     u,
		conn:    conn,
		Events:  make(chan MarketEvent, 256),
		waiters: map[int]chan socketReply{},
		closed:  make(chan struct{}),
	}
	go s.read()
	return s, nil
}

// Subscribe asks for the current value of each stream, then live updates.
// One bad name subscribes nothing.
func (s *MarketSocket) Subscribe(streams []string) error {
	if len(streams) == 0 {
		return nil
	}
	return s.roundTrip("SUBSCRIBE", streams)
}

// Unsubscribe drops streams. Unknown names are ignored by the server.
func (s *MarketSocket) Unsubscribe(streams []string) error {
	if len(streams) == 0 {
		return nil
	}
	return s.roundTrip("UNSUBSCRIBE", streams)
}

// Close ends the connection.
func (s *MarketSocket) Close() {
	s.once.Do(func() {
		close(s.closed)
		s.conn.Close()
	})
}

func (s *MarketSocket) roundTrip(method string, params []string) error {
	s.write.Lock()
	s.next++
	id := s.next
	ch := make(chan socketReply, 1)
	s.mu.Lock()
	s.waiters[id] = ch
	s.mu.Unlock()
	frame := map[string]any{"method": method, "params": params, "id": id}
	s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := s.conn.WriteJSON(frame)
	s.write.Unlock()
	if err != nil {
		s.dropWaiter(id)
		return err
	}
	select {
	case rep := <-ch:
		if rep.code != 0 {
			return fmt.Errorf("kyber market socket %s: %s", method, rep.msg)
		}
		return nil
	case <-time.After(8 * time.Second):
		s.dropWaiter(id)
		return fmt.Errorf("kyber market socket %s: no reply", method)
	case <-s.closed:
		return fmt.Errorf("kyber market socket closed")
	}
}

func (s *MarketSocket) dropWaiter(id int) {
	s.mu.Lock()
	delete(s.waiters, id)
	s.mu.Unlock()
}

func (s *MarketSocket) read() {
	defer close(s.Events)
	defer s.Close()
	for {
		s.conn.SetReadDeadline(time.Now().Add(45 * time.Second))
		_, msg, err := s.conn.ReadMessage()
		if err == nil {
			debugAppend("FRAME "+s.url, msg)
		}
		if err != nil {
			select {
			case <-s.closed:
			case s.Events <- MarketEvent{Err: err}:
			}
			return
		}
		ev, rep, isReply, ok := decodeFrame(msg)
		if !ok {
			continue
		}
		if isReply {
			s.mu.Lock()
			ch := s.waiters[rep.id]
			delete(s.waiters, rep.id)
			s.mu.Unlock()
			if ch != nil {
				ch <- rep.reply
			}
			continue
		}
		select {
		case <-s.closed:
			return
		case s.Events <- ev:
		}
	}
}

type decodedReply struct {
	id    int
	reply socketReply
}

func decodeFrame(msg []byte) (MarketEvent, decodedReply, bool, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(msg, &raw); err != nil {
		return MarketEvent{}, decodedReply{}, false, false
	}
	if _, ok := raw["e"]; !ok {
		id, _ := rawInt(raw, "id")
		code, hasCode := rawInt(raw, "code")
		rep := socketReply{}
		if hasCode && code != 0 {
			rep.code = int(code)
			rep.msg, _ = rawString(raw, "msg")
			if rep.msg == "" {
				rep.msg = "subscription rejected"
			}
		}
		return MarketEvent{}, decodedReply{id: int(id), reply: rep}, true, true
	}
	kind, _ := rawString(raw, "e")
	ev := MarketEvent{Kind: kind}
	ev.Time, _ = rawInt(raw, "E")
	switch kind {
	case "heartbeat":
		return ev, decodedReply{}, false, true
	case "status":
		c, _ := rawString(raw, "c")
		ev.ChainID, _ = strconv.Atoi(c)
		ev.StatusKind, _ = rawString(raw, "k")
		ev.Degraded, _ = rawBool(raw, "d")
		return ev, decodedReply{}, false, true
	case "onchainPrice":
		fillStream(&ev, raw)
		if v, ok := rawString(raw, "bp"); ok {
			ev.HasBuy = true
			ev.Buy = v
		}
		ev.BuyAt, _ = rawInt(raw, "bt")
		if v, ok := rawString(raw, "sp"); ok {
			ev.HasSell = true
			ev.Sell = v
		}
		ev.SellAt, _ = rawInt(raw, "st")
		return ev, decodedReply{}, false, true
	case "settlementPrice":
		fillStream(&ev, raw)
		ev.Settlement, _ = rawString(raw, "p")
		ev.Epoch, _ = rawInt(raw, "ep")
		ev.Block, _ = rawInt(raw, "bn")
		return ev, decodedReply{}, false, true
	default:
		return MarketEvent{}, decodedReply{}, false, false
	}
}

func fillStream(ev *MarketEvent, raw map[string]json.RawMessage) {
	s, _ := rawString(raw, "s")
	chain, addr, ok := strings.Cut(s, ":")
	if !ok {
		return
	}
	ev.ChainID, _ = strconv.Atoi(chain)
	ev.Address = addr
}

func rawString(m map[string]json.RawMessage, k string) (string, bool) {
	v, ok := m[k]
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return "", false
	}
	return s, true
}

func rawInt(m map[string]json.RawMessage, k string) (int64, bool) {
	v, ok := m[k]
	if !ok {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(v, &n); err == nil {
		return n, true
	}
	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		return 0, false
	}
	return int64(f), true
}

func rawBool(m map[string]json.RawMessage, k string) (bool, bool) {
	v, ok := m[k]
	if !ok {
		return false, false
	}
	var b bool
	if err := json.Unmarshal(v, &b); err != nil {
		return false, false
	}
	return b, true
}
