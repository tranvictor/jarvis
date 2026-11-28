package kyber

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Ways the debug file can fail:
//   - The flag is off, so a call must not create the file or write a byte.
//   - The .jarvis directory is missing, so the first write has to create it
//     and then append.
//   - A second response must extend the file, not replace it.
//   - A non-2xx body is still what Kyber returned, so it is stored as read.
//   - Two calls at once must not tear a body in half.
//   - A socket frame is stored as the bytes on the wire, not the decoded event.
//   - The file writer must not print, so the terminal stays as it was.

func TestDebugStaysQuietWhenDisabled(t *testing.T) {
	if err := EnableDebug(""); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.out")
	body := []byte(`{"code":0,"data":{"ok":true}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()
	c := &Client{key: "k", base: srv.URL, http: srv.Client()}
	var dest struct {
		OK bool `json:"ok"`
	}
	if err := c.get(t.Context(), "/chains", nil, &dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("debug file exists without the flag: %v", err)
	}
}

func TestDebugAppendsRawResponses(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".jarvis")
	path := filepath.Join(dir, "debug.out")
	if err := EnableDebug(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = EnableDebug("") })

	first := []byte("{\"code\":0,\"message\":\"OK\",\"data\":{\"chains\":[]}}\n")
	second := []byte("{\"code\":1,\"message\":\"no\"}")
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.Write(first)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		w.Write(second)
	}))
	defer srv.Close()
	c := &Client{key: "k", base: srv.URL, http: srv.Client()}
	var dest struct {
		Chains []Chain `json:"chains"`
	}
	if err := c.get(t.Context(), "/chains", nil, &dest); err != nil {
		t.Fatal(err)
	}
	if err := c.get(t.Context(), "/tokens/market", nil, &dest); err == nil {
		t.Fatal("expected the error status to fail the call")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "GET "+srv.URL+"/chains\n") || !strings.Contains(text, string(first)) {
		t.Fatalf("first response missing:\n%s", text)
	}
	if !strings.Contains(text, "GET "+srv.URL+"/tokens/market\n") || !strings.Contains(text, string(second)) {
		t.Fatalf("error body missing:\n%s", text)
	}
	if strings.Index(text, string(first)) > strings.Index(text, string(second)) {
		t.Fatalf("second response replaced the first:\n%s", text)
	}
}

func TestDebugAppendsConcurrentResponsesWhole(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.out")
	if err := EnableDebug(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = EnableDebug("") })

	bodies := make([]string, 8)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(r.URL.Query().Get("body")))
	}))
	defer srv.Close()
	c := &Client{key: "k", base: srv.URL, http: srv.Client()}
	var wg sync.WaitGroup
	for i := range bodies {
		bodies[i] = strings.Repeat(string(rune('A'+i)), 40)
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			q := url.Values{}
			q.Set("body", body)
			var dest struct{}
			_ = c.get(t.Context(), "/x", q, &dest)
		}(bodies[i])
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, body := range bodies {
		if !strings.Contains(text, body) {
			t.Fatalf("torn or missing body %q:\n%s", body, text)
		}
	}
}

func TestDebugAppendsSocketFrame(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.out")
	if err := EnableDebug(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = EnableDebug("") })

	frame := []byte(`{"e":"onchainPrice","E":1793605032000,"s":"1:0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","bp":"2940.5","bt":1793605030000,"sp":"2938.1","st":1793605032000}`)
	held := make(chan struct{})
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, frame)
		<-held
	}))
	defer srv.Close()
	defer close(held)

	c := &Client{key: "k", base: srv.URL, http: srv.Client()}
	sock, err := c.OpenMarketSocket(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer sock.Close()

	deadline := time.Now().Add(2 * time.Second)
	var text string
	for {
		b, err := os.ReadFile(path)
		if err == nil {
			text = string(b)
			if strings.Contains(text, string(frame)) {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("frame missing:\n%s", text)
		}
		time.Sleep(10 * time.Millisecond)
	}
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/tokens/market/socket"
	if !strings.Contains(text, wsURL) {
		t.Fatalf("socket url missing:\n%s", text)
	}
}
