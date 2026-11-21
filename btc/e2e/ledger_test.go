//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
)

const ledgerMnemonic = "legal winner thank year wave sausage worth useful legal winner thank yellow"

// speculos is a Ledger device emulator running the real Bitcoin Test app.
type speculos struct {
	t        *testing.T
	cmd      *exec.Cmd
	apdu     int
	api      int
	mu       sync.Mutex
	mode     string
	stop     chan struct{}
	screens  strings.Builder
	lastText string
}

func startSpeculos(t *testing.T, dir, mnemonic string, tag string) *speculos {
	bin := os.Getenv("SPECULOS_BIN")
	elf := os.Getenv("LEDGER_APP_ELF")
	if bin == "" || elf == "" {
		return nil
	}
	level := os.Getenv("SPECULOS_API_LEVEL")
	if level == "" {
		level = "27"
	}
	s := &speculos{t: t, apdu: freePort(t), api: freePort(t), mode: "yes", stop: make(chan struct{})}
	s.cmd = exec.Command(bin, "--model", "nanosp", "-a", level, "--display", "headless",
		"--apdu-port", fmt.Sprint(s.apdu), "--api-port", fmt.Sprint(s.api), "--seed", mnemonic, elf)
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logf, _ := os.Create(filepath.Join(dir, "speculos-"+tag+".log"))
	s.cmd.Stdout, s.cmd.Stderr = logf, logf
	if err := s.cmd.Start(); err != nil {
		t.Fatalf("starting Speculos: %v", err)
	}
	t.Cleanup(s.close)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if txt, err := s.currentText(); err == nil && strings.Contains(txt, "ready") {
			go s.drive()
			return s
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("Speculos did not come up (see speculos log)")
	return nil
}

func (s *speculos) addr() string { return fmt.Sprintf("127.0.0.1:%d", s.apdu) }

func (s *speculos) get(path string, out any) error {
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", s.api, path))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (s *speculos) button(which string) {
	body := bytes.NewReader([]byte(`{"action":"press-and-release"}`))
	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/button/%s", s.api, which), "application/json", body)
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func (s *speculos) currentText() (string, error) {
	var ev struct {
		Events []struct {
			Text string `json:"text"`
		} `json:"events"`
	}
	if err := s.get("/events?currentscreenonly=true", &ev); err != nil {
		return "", err
	}
	parts := make([]string, 0, len(ev.Events))
	for _, e := range ev.Events {
		parts = append(parts, e.Text)
	}
	return strings.Join(parts, " | "), nil
}

// press sets what the helper does at each confirmation: "yes" approves,
// "no" rejects.
func (s *speculos) press(mode string) {
	s.mu.Lock()
	s.mode = mode
	s.screens.Reset()
	s.lastText = ""
	s.mu.Unlock()
}

func (s *speculos) seen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.screens.String()
}

var idleScreens = []string{"app is ready", "Application", "App info", "Quit", "Settings", "Version", "Developer", "Bitcoin Testnet", "Bitcoin Test"}

// drive walks the Nano S+ screens with the two buttons: right to page,
// both to select.
func (s *speculos) drive() {
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
		}
		txt, err := s.currentText()
		if err != nil || txt == "" {
			continue
		}
		idle := false
		for _, i := range idleScreens {
			if strings.Contains(txt, i) {
				idle = true
			}
		}
		if idle {
			continue
		}
		s.mu.Lock()
		mode := s.mode
		if txt != s.lastText {
			fmt.Fprintf(&s.screens, "screen: %s\n", txt)
			s.lastText = txt
		}
		s.mu.Unlock()
		approve := strings.Contains(txt, "Approve") || strings.Contains(txt, "Confirm") || strings.Contains(txt, "Accept") || strings.Contains(txt, "Sign transaction")
		reject := strings.Contains(txt, "Reject")
		switch {
		case mode == "yes" && approve && !reject:
			s.button("both")
		case mode == "no" && reject:
			s.button("both")
		default:
			s.button("right")
		}
	}
}

func (s *speculos) close() {
	select {
	case <-s.stop:
	default:
		close(s.stop)
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = s.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		}
		s.cmd = nil
	}
}

// tbAddr re-encodes a regtest address with the testnet prefix, which is how
// the Bitcoin Test app prints every non-mainnet address.
func tbAddr(t *testing.T, regtest string) string {
	a, err := btcutil.DecodeAddress(regtest, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatal(err)
	}
	h := a.ScriptAddress()
	tb, err := btcutil.NewAddressWitnessPubKeyHash(h, &chaincfg.TestNet3Params)
	if err != nil {
		t.Fatal(err)
	}
	return tb.EncodeAddress()
}

func (e *Env) ledgerScenarios(t *testing.T, dev *speculos, wrong *speculos) {
	const name = "hw-ledger"
	env := "JARVIS_LEDGER_SPECULOS=" + dev.addr()
	ext := func(i uint32) string { return hdAddr(t, ledgerMnemonic, 0, i) }
	chg := func(i uint32) string { return hdAddr(t, ledgerMnemonic, 1, i) }

	e.scenario("L01-ledger-add", "Adding a Ledger reads the account key, shows the first address on the device, and derives the same addresses as an independent BIP-84 implementation",
		[]string{"Bitcoin app GET_EXTENDED_PUBKEY", "GET_WALLET_ADDRESS display", "master fingerprint", "testnet prefix vs regtest"}, func(s *S) {
			s.env = []string{env, "JARVIS_BTC_DEVICE_WAIT=5s"}
			dev.press("yes")
			out, code := s.jarvis("", "btc", "wallet", "add", "--kind", "ledger", "--desc", name)
			s.expectOK(out, code)
			s.expectContains(out, ext(0))
			if !strings.Contains(squash(dev.seen()), squash(tbAddr(t, ext(0)))) {
				s.fail("the device never showed %s:\n%s", tbAddr(t, ext(0)), dev.seen())
			}
			s.note("device screens seen while adding:\n%s", dev.seen())
			rec, _ := os.ReadFile(filepath.Join(e.home, ".jarvis", "btc", name+".json"))
			if !strings.Contains(string(rec), "master_fingerprint") || !strings.Contains(string(rec), "tpub") {
				s.fail("record lacks fingerprint or tpub: %s", rec)
			}
			for _, bad := range []string{"tprv", "mnemonic", "legal winner"} {
				if strings.Contains(string(rec), bad) {
					s.fail("record contains %q", bad)
				}
			}
		})

	e.scenario("L02-ledger-send-multi-input", "Send 0.4 BTC from three funded addresses: the Bitcoin app signs every input; bitcoind validates the result",
		[]string{"sign_psbt with client-command interpreter", "per-input derivation", "change recognised by the device", "device shows destination, amount and fee"}, func(s *S) {
			s.env = []string{env, "JARVIS_BTC_DEVICE_WAIT=5s"}
			e.fund(ext(0), 0.2, true)
			e.fund(ext(3), 0.3, true)
			e.fund(ext(7), 0.1, false)
			out, code := s.jarvis("", "btc", "balance", name)
			s.expectOK(out, code)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.50000000") {
				s.fail("confirmed = %q", got)
			}
			e.mine(1)

			dev.press("yes")
			dest := minerDest(e)
			out, code = s.jarvis("", "btc", "send", "--from", name, "--to", dest, "--amount", "0.4", "--fee-rate", "5", "--yes")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid == "" {
				s.note("device screens:\n%s", dev.seen())
				return
			}
			d := e.decode(txid)
			if len(d.Vin) < 2 {
				s.fail("expected a multi-input spend, got %d inputs", len(d.Vin))
			}
			var toDest, toChange int64
			for _, o := range d.Vout {
				switch o.ScriptPubKey.Address {
				case dest:
					toDest += satOf(o.Value)
				case chg(0):
					toChange += satOf(o.Value)
				default:
					s.fail("unexpected output %s", o.ScriptPubKey.Address)
				}
			}
			if toDest != 40_000_000 || toChange == 0 {
				s.fail("destination got %d, change %d at %s", toDest, toChange, chg(0))
			}
			me := e.entry(txid)
			screens := squash(dev.seen())
			if !strings.Contains(screens, squash(tbAddr(t, dest))) {
				s.fail("the device never displayed the destination %s", tbAddr(t, dest))
			}
			if !strings.Contains(screens, "0.4") {
				s.fail("the device never displayed the amount")
			}
			feeOnDevice := strings.TrimRight(btcStr(satOf(me.Fees.Base)), "0")
			if !strings.Contains(screens, feeOnDevice) {
				s.fail("the device never displayed the fee %s", feeOnDevice)
			}
			s.note("txid %s, %d inputs, fee %d sat, change %d sat at %s", txid, len(d.Vin), satOf(me.Fees.Base), toChange, chg(0))
			s.note("device screens seen while signing:\n%s", dev.seen())
			e.mine(1)
		})

	e.scenario("L03-ledger-change-advances", "Second payment sends change to the next unused change address",
		[]string{"change index selection"}, func(s *S) {
			s.env = []string{env, "JARVIS_BTC_DEVICE_WAIT=5s"}
			dev.press("yes")
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", minerDest(e), "--amount", "0.05", "--fee-rate", "5", "--yes")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid == "" {
				return
			}
			found := false
			for _, o := range e.decode(txid).Vout {
				if o.ScriptPubKey.Address == chg(1) {
					found = true
				}
				if o.ScriptPubKey.Address == chg(0) {
					s.fail("change reused %s", chg(0))
				}
			}
			if !found {
				s.fail("no change at %s", chg(1))
			}
			e.mine(1)
		})

	e.scenario("L04-ledger-reject", "Rejecting on the device aborts the send; nothing is broadcast",
		[]string{"user reject on device"}, func(s *S) {
			s.env = []string{env, "JARVIS_BTC_DEVICE_WAIT=5s"}
			dev.press("no")
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", minerDest(e), "--amount", "0.01", "--fee-rate", "5", "--yes")
			s.expectFail(out, code, "cancelled", "nothing was broadcast")
			s.expectNotContains(out, "Broadcast ")
			s.expectMempool(0)
			s.note("device screens:\n%s", dev.seen())
		})

	e.scenario("L05-ledger-wrong-device", "A Ledger that is not the wallet's device is refused before anything is shown or signed",
		[]string{"master fingerprint mismatch"}, func(s *S) {
			s.env = []string{"JARVIS_LEDGER_SPECULOS=" + wrong.addr(), "JARVIS_BTC_DEVICE_WAIT=5s"}
			wrong.press("yes")
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", minerDest(e), "--amount", "0.01", "--fee-rate", "5", "--yes")
			s.expectFail(out, code, "fingerprint")
			s.expectNotContains(out, "Broadcast ")
			s.expectMempool(0)
			if strings.Contains(wrong.seen(), "Amount") {
				s.fail("the wrong device was asked to confirm a transaction:\n%s", wrong.seen())
			}
		})

	e.scenario("L06-ledger-sweep", "Sweeping the whole wallet signs every input on its own path",
		[]string{"sweep with many inputs"}, func(s *S) {
			s.env = []string{env, "JARVIS_BTC_DEVICE_WAIT=5s"}
			e.fund(ext(12), 0.05, true)
			dev.press("yes")
			dest := minerDest(e)
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", dest, "--amount", "ALL", "--fee-rate", "3", "--yes")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid != "" {
				d := e.decode(txid)
				if len(d.Vout) != 1 || d.Vout[0].ScriptPubKey.Address != dest {
					s.fail("sweep must have exactly one output to the destination")
				}
				if len(d.Vin) < 3 {
					s.fail("sweep spent only %d inputs", len(d.Vin))
				}
				s.note("sweep spent %d inputs", len(d.Vin))
			}
			e.mine(1)
			out, _ = s.jarvis("", "btc", "balance", name)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.00000000") {
				s.fail("wallet not empty after sweep: %q", got)
			}
		})

	e.scenario("L07-ledger-unplugged-send", "Sending with no device attached fails cleanly",
		[]string{"device missing"}, func(s *S) {
			e.fund(ext(0), 0.01, true)
			dev.close()
			s.env = []string{env, "JARVIS_BTC_DEVICE_WAIT=2s"}
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", minerDest(e), "--amount", "0.001", "--fee-rate", "5", "--yes")
			s.expectFail(out, code, "ledger")
			s.expectNotContains(out, "Broadcast ")
			s.expectMempool(0)
		})
}
