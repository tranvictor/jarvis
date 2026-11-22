package util

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/broadcaster"
)

// Ways a mev-protected broadcast can fail:
//
//  1. Untagged nodes are no longer contacted together, so one slow node
//     blocks the rest.
//  2. A mev-protected node accepts the tx and a normal node still receives it.
//  3. Several mev-protected nodes are tried one after another instead of together.
//  4. Normal nodes are contacted before every mev-protected node has rejected.
//  5. Every mev-protected node rejects, a normal node accepts, and the result
//     hides that the protected tier failed.
//  6. Both tiers reject, and the error drops one of the tiers.
//  7. One mev-protected node accepts, another rejects, and the normal tier is
//     still used.
//  8. A tag other than mev-protected opens a preferred tier.
//  9. "MEV-Protected" with surrounding space is not recognised.
//  10. A mev-protected node that cannot be dialed is waited on, or the
//     remaining nodes are skipped.
//  11. A broadcaster with no dialable nodes stops returning success.
//  12. An explicit empty tag list does not suppress a built-in mev-protected tag.
//  13. A built-in tag still applies after the node's URL is changed.
//  14. A node file without a tags field fails to load.
//  15. Tags saved in the node file are not the tags the broadcaster uses.

type mevHit struct {
	Node   string    `json:"node"`
	At     time.Time `json:"at"`
	Result string    `json:"result"`
}

type mevScenario struct {
	Name   string   `json:"name"`
	OK     bool     `json:"ok"`
	Error  string   `json:"error,omitempty"`
	Hits   []mevHit `json:"hits"`
	Passed bool     `json:"passed"`
	Note   string   `json:"note,omitempty"`
}

type mevReport struct {
	mu         sync.Mutex
	Scenarios  []mevScenario `json:"scenarios"`
	NodeConfig string        `json:"node_config,omitempty"`
}

func (r *mevReport) add(s mevScenario) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Scenarios = append(r.Scenarios, s)
}

func TestMEVProtectedBroadcast(t *testing.T) {
	dir := t.TempDir()
	nodeConfigDirForTest = func() (string, error) {
		p := filepath.Join(dir, "nodes")
		if err := os.MkdirAll(p, 0o755); err != nil {
			return "", err
		}
		return p, nil
	}
	t.Cleanup(func() { nodeConfigDirForTest = nil })
	t.Setenv("ETHEREUM_MAINNET_NODE", "")
	t.Setenv("BSC_MAINNET_NODE", "")
	t.Setenv("BASE_MAINNET_NODE", "")

	report := &mevReport{}
	t.Cleanup(func() {
		path := "/opt/cursor/artifacts/mev-broadcast-report.json"
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Errorf("artifact dir: %s", err)
			return
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Errorf("artifact json: %s", err)
			return
		}
		if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
			t.Errorf("write artifact: %s", err)
		}
	})

	t.Run("kyber-protection is not tagged", func(t *testing.T) {
		sc := mevScenario{Name: "kyber-protection has no built-in mev-protected tag"}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		for _, n := range []networks.Network{networks.EthereumMainnet, networks.BSCMainnet, networks.BaseMainnet} {
			if broadcaster.HasMEVTag(n.GetDefaultNodeTags()["kyber-protection"]) {
				t.Fatalf("%s tags kyber-protection by default: %v", n.GetName(), n.GetDefaultNodeTags())
			}
			url := n.GetDefaultNodes()["kyber-protection"]
			got := EffectiveTags("kyber-protection", url, nil, n.GetDefaultNodeTags(), n.GetDefaultNodes())
			if broadcaster.HasMEVTag(got) {
				t.Fatalf("%s effective tags = %v", n.GetName(), got)
			}
		}
		defaults := map[string][]string{"private": {broadcaster.TagMEVProtected}}
		nodes := map[string]string{"private": "https://private.example/rpc"}
		if !broadcaster.HasMEVTag(EffectiveTags("private", nodes["private"], nil, defaults, nodes)) {
			t.Fatal("a configured default tag was dropped")
		}
		if broadcaster.HasMEVTag(EffectiveTags("private", "https://other.example", nil, defaults, nodes)) {
			t.Fatal("a default tag survived a URL change")
		}
		cleared := EffectiveTags("private", nodes["private"], map[string][]string{"private": {}}, defaults, nodes)
		if len(cleared) != 0 {
			t.Fatalf("explicit empty tags = %v", cleared)
		}
	})

	t.Run("legacy node file", func(t *testing.T) {
		sc := mevScenario{Name: "node file without tags"}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		var cfg NodeConfig
		raw := []byte(`{"nodes":{"public":"https://public.example"},"use_defaults":false}`)
		if err := json.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Tags != nil {
			t.Fatalf("tags = %#v", cfg.Tags)
		}
		if cfg.Nodes["public"] != "https://public.example" || cfg.UseDefaults {
			t.Fatalf("cfg = %+v", cfg)
		}
		if err := SaveNodeConfig("legacy-net", cfg); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadNodeConfig("legacy-net")
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Tags != nil || loaded.Nodes["public"] != cfg.Nodes["public"] {
			t.Fatalf("reloaded = %+v", loaded)
		}
	})

	t.Run("tag helpers", func(t *testing.T) {
		sc := mevScenario{Name: "tag normalize, union, subtract"}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		got := UnionTags([]string{" fast "}, []string{"MEV-Protected", "mev-protected"})
		if len(got) != 2 || got[0] != "fast" || got[1] != broadcaster.TagMEVProtected {
			t.Fatalf("union = %v", got)
		}
		left := SubtractTags([]string{"mev-protected", "fast"}, []string{" MEV-PROTECTED "})
		if len(left) != 1 || left[0] != "fast" {
			t.Fatalf("subtract = %v", left)
		}
		if !SameTags([]string{"MEV-Protected", "fast"}, []string{"fast", "mev-protected"}) {
			t.Fatal("same tags differed")
		}
		empty := SubtractTags([]string{"mev-protected"}, []string{"mev-protected"})
		if empty == nil || len(empty) != 0 {
			t.Fatalf("empty subtract = %#v", empty)
		}
	})

	t.Run("untagged nodes together", func(t *testing.T) {
		var hits []mevHit
		var mu sync.Mutex
		together := true
		release := make(chan struct{})
		var once sync.Once
		arrived := 0
		srv := map[string]*httptest.Server{}
		newNode := func(name string) *httptest.Server {
			return httptest.NewServer(rpcHandler(func() (bool, string) {
				mu.Lock()
				arrived++
				n := arrived
				hits = append(hits, mevHit{Node: name, At: time.Now(), Result: "entered"})
				mu.Unlock()
				if n == 2 {
					once.Do(func() { close(release) })
				}
				select {
				case <-release:
				case <-time.After(1500 * time.Millisecond):
					mu.Lock()
					together = false
					mu.Unlock()
				}
				return true, "ok"
			}))
		}
		srv["a"] = newNode("a")
		srv["b"] = newNode("b")
		t.Cleanup(func() {
			srv["a"].Close()
			srv["b"].Close()
		})
		bc := broadcaster.NewGenericBroadcaster(map[string]string{
			"a": srv["a"].URL,
			"b": srv["b"].URL,
		}, map[string][]string{"a": {"fast"}})
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc := mevScenario{Name: "untagged nodes are contacted together", OK: ok, Hits: hits}
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			sc.Note = "a tag other than mev-protected does not create a preferred tier"
			report.add(sc)
		}()
		if !ok || err != nil {
			t.Fatalf("broadcast ok=%v err=%v", ok, err)
		}
		if !together {
			t.Fatal("nodes were not contacted together")
		}
		if len(hits) != 2 {
			t.Fatalf("hits = %+v", hits)
		}
	})

	t.Run("protected accepts and normal is not used", func(t *testing.T) {
		hits, nodes := rpcNodes(t, map[string]func() (bool, string){
			"protected": func() (bool, string) { return true, "ok" },
			"public":    func() (bool, string) { return true, "ok" },
		})
		bc := broadcaster.NewGenericBroadcaster(nodes, map[string][]string{
			"protected": {"  MEV-Protected  "},
		})
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc := mevScenario{Name: "mev-protected acceptance skips normal nodes", OK: ok, Hits: snapshotHits(hits)}
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		if !ok || err != nil {
			t.Fatalf("broadcast ok=%v err=%v", ok, err)
		}
		if countNode(sc.Hits, "public") != 0 || countNode(sc.Hits, "protected") != 1 {
			t.Fatalf("hits = %+v", sc.Hits)
		}
	})

	t.Run("protected tier is parallel and normal waits", func(t *testing.T) {
		var mu sync.Mutex
		var hits []mevHit
		release := make(chan struct{})
		var once sync.Once
		arrived := 0
		record := func(name, result string) {
			mu.Lock()
			hits = append(hits, mevHit{Node: name, At: time.Now(), Result: result})
			mu.Unlock()
		}
		protected := func(name string) http.Handler {
			return rpcHandler(func() (bool, string) {
				record(name, "start")
				mu.Lock()
				arrived++
				n := arrived
				mu.Unlock()
				if n == 2 {
					once.Do(func() { close(release) })
				}
				select {
				case <-release:
				case <-time.After(1500 * time.Millisecond):
					t.Errorf("%s was not contacted with the other protected node", name)
				}
				record(name, "done")
				return false, "rejected"
			})
		}
		var normalAt time.Time
		servers := map[string]*httptest.Server{
			"p1": httptest.NewServer(protected("p1")),
			"p2": httptest.NewServer(protected("p2")),
			"public": httptest.NewServer(rpcHandler(func() (bool, string) {
				normalAt = time.Now()
				record("public", "ok")
				return true, "ok"
			})),
		}
		t.Cleanup(func() {
			for _, s := range servers {
				s.Close()
			}
		})
		bc := broadcaster.NewGenericBroadcaster(map[string]string{
			"p1":     servers["p1"].URL,
			"p2":     servers["p2"].URL,
			"public": servers["public"].URL,
		}, map[string][]string{
			"p1": {broadcaster.TagMEVProtected},
			"p2": {broadcaster.TagMEVProtected},
		})
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc := mevScenario{Name: "protected nodes together, then normal fallback", OK: ok, Hits: append([]mevHit(nil), hits...)}
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		if !ok {
			t.Fatalf("broadcast failed: %v", err)
		}
		if !errors.Is(err, broadcaster.ErrMEVFallback) {
			t.Fatalf("err = %v", err)
		}
		var done []time.Time
		for _, hit := range hits {
			if hit.Result == "done" {
				done = append(done, hit.At)
			}
		}
		if len(done) != 2 || normalAt.IsZero() {
			t.Fatalf("hits = %+v", hits)
		}
		for _, at := range done {
			if normalAt.Before(at) {
				t.Fatalf("public node was contacted at %s before a protected node finished at %s", normalAt, at)
			}
		}
	})

	t.Run("one protected acceptance is enough", func(t *testing.T) {
		hits, nodes := rpcNodes(t, map[string]func() (bool, string){
			"good":   func() (bool, string) { return true, "ok" },
			"bad":    func() (bool, string) { return false, "rejected" },
			"public": func() (bool, string) { return true, "ok" },
		})
		bc := broadcaster.NewGenericBroadcaster(nodes, map[string][]string{
			"good": {broadcaster.TagMEVProtected},
			"bad":  {broadcaster.TagMEVProtected},
		})
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc := mevScenario{Name: "one protected acceptance skips the normal tier", OK: ok, Hits: snapshotHits(hits)}
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		if !ok || err != nil {
			t.Fatalf("broadcast ok=%v err=%v", ok, err)
		}
		if countNode(sc.Hits, "public") != 0 {
			t.Fatalf("public node was used: %+v", sc.Hits)
		}
	})

	t.Run("both tiers reject", func(t *testing.T) {
		hits, nodes := rpcNodes(t, map[string]func() (bool, string){
			"protected": func() (bool, string) { return false, "protected-down" },
			"public":    func() (bool, string) { return false, "public-down" },
		})
		bc := broadcaster.NewGenericBroadcaster(nodes, map[string][]string{
			"protected": {broadcaster.TagMEVProtected},
		})
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc := mevScenario{Name: "both tiers reject", OK: ok, Hits: snapshotHits(hits)}
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		if ok || err == nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
		if errors.Is(err, broadcaster.ErrMEVFallback) {
			t.Fatalf("fallback notice on a failed broadcast: %v", err)
		}
		msg := err.Error()
		if !containsAll(msg, "protected", "public", "protected-down", "public-down") {
			t.Fatalf("err = %s", msg)
		}
	})

	t.Run("undialable protected node is skipped", func(t *testing.T) {
		hits, nodes := rpcNodes(t, map[string]func() (bool, string){
			"public": func() (bool, string) { return true, "ok" },
		})
		nodes["ghost"] = filepath.Join(t.TempDir(), "missing.ipc")
		bc := broadcaster.NewGenericBroadcaster(nodes, map[string][]string{
			"ghost": {broadcaster.TagMEVProtected},
		})
		start := time.Now()
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc := mevScenario{Name: "undialable protected node is not available", OK: ok, Hits: snapshotHits(hits)}
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			sc.Note = "elapsed " + time.Since(start).String()
			report.add(sc)
		}()
		if !ok || err != nil {
			t.Fatalf("broadcast ok=%v err=%v", ok, err)
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("waited %s on a node that could not be dialed", time.Since(start))
		}
		if countNode(sc.Hits, "public") != 1 {
			t.Fatalf("hits = %+v", sc.Hits)
		}
	})

	t.Run("no dialable nodes", func(t *testing.T) {
		sc := mevScenario{Name: "no dialable nodes still reports success"}
		bc := broadcaster.NewGenericBroadcaster(nil, nil)
		_, ok, err := bc.Broadcast("0xdeadbeef")
		sc.OK = ok
		if err != nil {
			sc.Error = err.Error()
		}
		defer func() {
			sc.Passed = !t.Failed()
			report.add(sc)
		}()
		if !ok || err != nil {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})

	t.Run("node file selects the protected tier", func(t *testing.T) {
		var mode string
		var mu sync.Mutex
		hits, nodes := rpcNodes(t, map[string]func() (bool, string){
			"protected": func() (bool, string) {
				mu.Lock()
				defer mu.Unlock()
				if mode == "reject" {
					return false, "protected-down"
				}
				return true, "ok"
			},
			"public": func() (bool, string) { return true, "ok" },
		})
		cfg := NodeConfig{
			Nodes:       nodes,
			UseDefaults: false,
			Tags: map[string][]string{
				"protected": {broadcaster.TagMEVProtected},
			},
		}
		if err := SaveNodeConfig("mainnet", cfg); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "nodes", "mainnet.json"))
		if err != nil {
			t.Fatal(err)
		}
		report.mu.Lock()
		report.NodeConfig = string(raw)
		report.mu.Unlock()

		loaded, err := GetNodes(networks.EthereumMainnet)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded) != 2 {
			t.Fatalf("active nodes = %v", loaded)
		}
		tags := EffectiveTagMap(networks.EthereumMainnet, loaded)
		if !broadcaster.HasMEVTag(tags["protected"]) || len(tags["public"]) != 0 {
			t.Fatalf("tags = %v", tags)
		}
		bc := broadcaster.NewGenericBroadcaster(loaded, tags)

		_, ok, err := bc.Broadcast("0xdeadbeef")
		first := mevScenario{Name: "config file: protected node accepts", OK: ok, Hits: snapshotHits(hits)}
		if err != nil {
			first.Error = err.Error()
		}
		if !ok || err != nil || countNode(first.Hits, "public") != 0 || countNode(first.Hits, "protected") != 1 {
			first.Passed = false
			report.add(first)
			t.Fatalf("first broadcast ok=%v err=%v hits=%+v", ok, err, first.Hits)
		}
		first.Passed = true
		report.add(first)

		mu.Lock()
		mode = "reject"
		mu.Unlock()
		_, ok, err = bc.Broadcast("0xdeadbeef")
		second := mevScenario{Name: "config file: protected node fails, public fallback", OK: ok, Hits: snapshotHits(hits)}
		if err != nil {
			second.Error = err.Error()
		}
		defer func() {
			second.Passed = !t.Failed()
			report.add(second)
		}()
		if !ok || !errors.Is(err, broadcaster.ErrMEVFallback) {
			t.Fatalf("fallback ok=%v err=%v", ok, err)
		}
		if countNode(second.Hits, "public") != 1 || countNode(second.Hits, "protected") != 2 {
			t.Fatalf("hits = %+v", second.Hits)
		}
		reloaded, err := LoadNodeConfig("mainnet")
		if err != nil {
			t.Fatal(err)
		}
		if !broadcaster.HasMEVTag(reloaded.Tags["protected"]) {
			t.Fatalf("saved tags = %v", reloaded.Tags)
		}
	})
}

func snapshotHits(hits *[]mevHit) []mevHit {
	out := append([]mevHit(nil), (*hits)...)
	return out
}

func countNode(hits []mevHit, name string) int {
	n := 0
	for _, hit := range hits {
		if hit.Node == name {
			n++
		}
	}
	return n
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !contains(s, part) {
			return false
		}
	}
	return true
}

func contains(s, part string) bool {
	return len(part) == 0 || (len(s) >= len(part) && (s == part || len(s) > 0 && (indexOf(s, part) >= 0)))
}

func indexOf(s, part string) int {
	for i := 0; i+len(part) <= len(s); i++ {
		if s[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}

func rpcNodes(t *testing.T, behavior map[string]func() (bool, string)) (*[]mevHit, map[string]string) {
	t.Helper()
	hits := &[]mevHit{}
	var mu sync.Mutex
	nodes := make(map[string]string, len(behavior))
	for name, fn := range behavior {
		name, fn := name, fn
		srv := httptest.NewServer(rpcHandler(func() (bool, string) {
			ok, result := fn()
			mu.Lock()
			*hits = append(*hits, mevHit{Node: name, At: time.Now(), Result: result})
			mu.Unlock()
			return ok, result
		}))
		t.Cleanup(srv.Close)
		nodes[name] = srv.URL
	}
	return hits, nodes
}

func rpcHandler(decide func() (bool, string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		ok, result := decide()
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}
		if ok {
			resp["result"] = "0x1"
		} else {
			resp["error"] = map[string]any{"code": -32000, "message": result}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
}
