//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tranvictor/jarvis/btc/e2e/shim"
)

// Step is one recorded jarvis (or bitcoind) invocation.
type Step struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

type Scenario struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Covers  []string `json:"covers"`
	Passed  bool     `json:"passed"`
	Skipped bool     `json:"skipped,omitempty"`
	Steps   []Step   `json:"steps"`
	Notes   []string `json:"notes,omitempty"`
	Fails   []string `json:"failures,omitempty"`
}

type Report struct {
	Started   time.Time  `json:"started"`
	Bitcoind  string     `json:"bitcoind"`
	GoVersion string     `json:"go"`
	Jarvis    string     `json:"jarvis_binary_sha256"`
	Scenarios []Scenario `json:"scenarios"`
}

type Env struct {
	t        *testing.T
	dir      string
	home     string
	bin      string
	rpc      *shim.RPC
	shim     *shim.Shim
	server   *httptest.Server
	esplora  string
	miner    string
	minerKey string
	report   *Report
	cmd      *exec.Cmd
	cli      string
	datadir  string
	rpcPort  int
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func findBitcoind(t *testing.T) string {
	if p := os.Getenv("BITCOIND"); p != "" {
		return p
	}
	if p, err := exec.LookPath("bitcoind"); err == nil {
		return p
	}
	t.Fatalf("bitcoind not found. Run scripts/e2e-btc.sh (it downloads a pinned Bitcoin Core) or set BITCOIND=/path/to/bitcoind")
	return ""
}

func repoRoot(t *testing.T) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func newEnv(t *testing.T) *Env {
	e := &Env{t: t, report: &Report{Started: time.Now().UTC(), GoVersion: runtime.Version()}}
	e.dir = t.TempDir()
	e.home = filepath.Join(e.dir, "home")
	if err := os.MkdirAll(e.home, 0o755); err != nil {
		t.Fatal(err)
	}

	e.bin = filepath.Join(e.dir, "jarvis")
	build := exec.Command("go", "build", "-o", e.bin, ".")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building jarvis: %v\n%s", err, out)
	}
	e.report.Jarvis = fileSHA256(t, e.bin)

	bitcoind := findBitcoind(t)
	e.datadir = filepath.Join(e.dir, "bitcoind")
	if err := os.MkdirAll(e.datadir, 0o755); err != nil {
		t.Fatal(err)
	}
	e.rpcPort = freePort(t)
	e.cmd = exec.Command(bitcoind, "-regtest", "-datadir="+e.datadir,
		"-rpcuser=e2e", "-rpcpassword=e2e", fmt.Sprintf("-rpcport=%d", e.rpcPort),
		"-listen=0", "-txindex=1", "-fallbackfee=0.0002", "-server=1", "-printtoconsole=0")
	if err := e.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e.cmd.Process != nil {
			_, _ = e.rpc.Call("stop")
			done := make(chan struct{})
			go func() { _ = e.cmd.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(15 * time.Second):
				_ = e.cmd.Process.Kill()
			}
		}
	})
	e.rpc = shim.NewRPC(fmt.Sprintf("http://127.0.0.1:%d", e.rpcPort), "e2e", "e2e")
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := e.rpc.Call("getblockchaininfo"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bitcoind did not start")
		}
		time.Sleep(200 * time.Millisecond)
	}
	var netinfo struct {
		Subversion string `json:"subversion"`
	}
	_ = e.rpc.CallInto(&netinfo, "getnetworkinfo")
	e.report.Bitcoind = strings.Trim(netinfo.Subversion, "/")

	if _, err := e.rpc.Call("createwallet", "miner"); err != nil {
		t.Fatal(err)
	}
	e.miner = e.walletAddr("miner")
	e.mine(101)

	e.shim = shim.New(e.rpc)
	e.shim.SetFees(map[int]float64{1: 20, 2: 10, 3: 8, 6: 4, 12: 2, 24: 1.5, 144: 1})
	e.server = httptest.NewServer(e.shim)
	t.Cleanup(e.server.Close)
	e.esplora = e.server.URL
	return e
}

func fileSHA256(t *testing.T, path string) string {
	out, err := exec.Command("sha256sum", path).Output()
	if err != nil {
		return "unavailable"
	}
	return strings.Fields(string(out))[0]
}

func (e *Env) walletRPC(wallet string) *shim.RPC {
	return shim.NewRPC(fmt.Sprintf("http://127.0.0.1:%d/wallet/%s", e.rpcPort, wallet), "e2e", "e2e")
}

func (e *Env) walletAddr(wallet string) string {
	var addr string
	if err := e.walletRPC(wallet).CallInto(&addr, "getnewaddress", "", "bech32"); err != nil {
		e.t.Fatal(err)
	}
	return addr
}

func (e *Env) mine(n int) {
	var hashes []string
	if err := e.rpc.CallInto(&hashes, "generatetoaddress", n, e.miner); err != nil {
		e.t.Fatal(err)
	}
}

// fund sends btc from the miner wallet to addr and returns the txid.
func (e *Env) fund(addr string, btc float64, confirm bool) string {
	var txid string
	if err := e.walletRPC("miner").CallInto(&txid, "sendtoaddress", addr, btc); err != nil {
		e.t.Fatalf("funding %s: %v", addr, err)
	}
	if confirm {
		e.mine(1)
	}
	return txid
}

func (e *Env) mempool() []string {
	var ids []string
	if err := e.rpc.CallInto(&ids, "getrawmempool"); err != nil {
		e.t.Fatal(err)
	}
	return ids
}

type mempoolEntry struct {
	VSize int `json:"vsize"`
	Fees  struct {
		Base float64 `json:"base"`
	} `json:"fees"`
}

func (e *Env) entry(txid string) mempoolEntry {
	var m mempoolEntry
	if err := e.rpc.CallInto(&m, "getmempoolentry", txid); err != nil {
		e.t.Fatalf("getmempoolentry %s: %v", txid, err)
	}
	return m
}

type decodedTx struct {
	Txid string `json:"txid"`
	Vout []struct {
		Value        float64 `json:"value"`
		ScriptPubKey struct {
			Address string `json:"address"`
		} `json:"scriptPubKey"`
	} `json:"vout"`
	Vin []struct {
		Txid string `json:"txid"`
		Vout int    `json:"vout"`
	} `json:"vin"`
}

func (e *Env) decode(txid string) decodedTx {
	var d decodedTx
	if err := e.rpc.CallInto(&d, "getrawtransaction", txid, true); err != nil {
		e.t.Fatalf("getrawtransaction %s: %v", txid, err)
	}
	return d
}

// S is the handle a scenario uses to run commands and state expectations.
type S struct {
	e   *Env
	t   *testing.T
	sc  *Scenario
	env []string
}

func (e *Env) scenario(id, title string, covers []string, fn func(s *S)) {
	e.t.Run(id, func(t *testing.T) {
		sc := Scenario{ID: id, Title: title, Covers: covers}
		s := &S{e: e, t: t, sc: &sc}
		fn(s)
		sc.Skipped = t.Skipped()
		sc.Passed = !t.Failed() && !sc.Skipped
		e.report.Scenarios = append(e.report.Scenarios, sc)
	})
}

func (s *S) note(format string, args ...any) {
	s.sc.Notes = append(s.sc.Notes, fmt.Sprintf(format, args...))
}

func (s *S) fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.sc.Fails = append(s.sc.Fails, msg)
	s.t.Errorf("%s", msg)
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// jarvis runs the binary against the regtest node and the shim.
func (s *S) jarvis(stdin string, args ...string) (string, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.e.bin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+s.e.home,
		"JARVIS_BTC_CHAIN=regtest",
		"JARVIS_BTC_ESPLORA="+s.e.esplora,
		"NO_COLOR=1", "TERM=dumb",
	)
	cmd.Env = append(cmd.Env, s.env...)
	cmd.Stdin = strings.NewReader(stdin)
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	out := strings.TrimSpace(ansi.ReplaceAllString(buf.String(), ""))
	s.sc.Steps = append(s.sc.Steps, Step{
		Command: "jarvis " + strings.Join(redact(args), " "), ExitCode: code, Output: out,
	})
	return out, code
}

func redact(args []string) []string { return args }

func (s *S) expectOK(out string, code int) {
	s.t.Helper()
	if code != 0 {
		s.fail("expected exit 0, got %d:\n%s", code, out)
	}
}

func (s *S) expectFail(out string, code int, contains ...string) {
	s.t.Helper()
	if code == 0 {
		s.fail("expected a non-zero exit, got 0:\n%s", out)
	}
	for _, c := range contains {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(c)) {
			s.fail("output should mention %q:\n%s", c, out)
		}
	}
}

func (s *S) expectContains(out string, parts ...string) {
	s.t.Helper()
	for _, p := range parts {
		if !strings.Contains(out, p) {
			s.fail("output should contain %q:\n%s", p, out)
		}
	}
}

func (s *S) expectNotContains(out string, parts ...string) {
	s.t.Helper()
	for _, p := range parts {
		if strings.Contains(out, p) {
			s.fail("output should not contain %q:\n%s", p, out)
		}
	}
}

func (s *S) expectMempool(n int) {
	s.t.Helper()
	if got := len(s.e.mempool()); got != n {
		s.fail("mempool has %d transactions, want %d: %v", got, n, s.e.mempool())
	}
}

var txidRE = regexp.MustCompile(`Broadcast ([0-9a-f]{64})`)

func (s *S) broadcastTxid(out string) string {
	m := txidRE.FindStringSubmatch(out)
	if m == nil {
		s.fail("no 'Broadcast <txid>' line in output:\n%s", out)
		return ""
	}
	return m[1]
}

func (s *S) writeReport() {}

func (e *Env) writeReport(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	js, _ := json.MarshalIndent(e.report, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "btc-e2e-report.json"), js, 0o644); err != nil {
		e.t.Fatal(err)
	}
	var md strings.Builder
	passed, skipped := 0, 0
	for _, sc := range e.report.Scenarios {
		if sc.Passed {
			passed++
		}
		if sc.Skipped {
			skipped++
		}
	}
	fmt.Fprintf(&md, "# Jarvis Bitcoin end-to-end report\n\n")
	fmt.Fprintf(&md, "- Run: %s\n- Node: %s (regtest, real consensus and policy validation of every broadcast)\n- Go: %s\n- jarvis binary sha256: `%s`\n- Result: **%d/%d scenarios passed, %d not run**\n\n",
		e.report.Started.Format(time.RFC3339), e.report.Bitcoind, e.report.GoVersion, e.report.Jarvis, passed, len(e.report.Scenarios), skipped)
	fmt.Fprintf(&md, "| Scenario | Result | Covers |\n|---|---|---|\n")
	for _, sc := range e.report.Scenarios {
		verdict := "PASS"
		if sc.Skipped {
			verdict = "NOT RUN"
		} else if !sc.Passed {
			verdict = "FAIL"
		}
		fmt.Fprintf(&md, "| %s: %s | %s | %s |\n", sc.ID, sc.Title, verdict, strings.Join(sc.Covers, ", "))
	}
	for _, sc := range e.report.Scenarios {
		fmt.Fprintf(&md, "\n## %s: %s\n\n", sc.ID, sc.Title)
		for _, n := range sc.Notes {
			fmt.Fprintf(&md, "- %s\n", n)
		}
		for _, f := range sc.Fails {
			fmt.Fprintf(&md, "- **FAILED:** %s\n", strings.SplitN(f, "\n", 2)[0])
		}
		for _, st := range sc.Steps {
			fmt.Fprintf(&md, "\n```\n$ %s   # exit %d\n%s\n```\n", st.Command, st.ExitCode, st.Output)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "btc-e2e-report.md"), []byte(md.String()), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

var _ = http.StatusOK
