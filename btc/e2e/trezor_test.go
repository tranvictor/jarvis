//go:build e2e

package e2e

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"crypto/sha512"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"golang.org/x/crypto/pbkdf2"
)

const trezorMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

// hdAddr derives m/84'/1'/0'/branch/index from the mnemonic with btcd, with
// no code shared with jarvis.
func hdAddr(t *testing.T, mnemonic string, branch, index uint32) string {
	seed := pbkdf2.Key([]byte(mnemonic), []byte("mnemonic"), 2048, 64, sha512.New)
	key, err := hdkeychain.NewMaster(seed, &chaincfg.RegressionNetParams)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []uint32{
		hdkeychain.HardenedKeyStart + 84, hdkeychain.HardenedKeyStart + 1, hdkeychain.HardenedKeyStart + 0, branch, index,
	} {
		if key, err = key.Derive(step); err != nil {
			t.Fatal(err)
		}
	}
	pub, err := key.ECPubKey()
	if err != nil {
		t.Fatal(err)
	}
	return regtestAddr(pub.SerializeCompressed())
}

type emulator struct {
	t        *testing.T
	port     int
	emu      *exec.Cmd
	profile  string
	python   string
	fwDir    string
	autoLog  string
	auto     *exec.Cmd
	autoMode string
}

func startEmulator(t *testing.T, dir string) *emulator {
	fw := os.Getenv("TREZOR_FIRMWARE_DIR")
	py := os.Getenv("TREZOR_PYTHON")
	if fw == "" || py == "" {
		return nil
	}
	e := &emulator{t: t, fwDir: fw, python: py, profile: filepath.Join(dir, "trezor-profile")}
	l, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	e.port = l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	e.emu = exec.Command(py, filepath.Join(fw, "core", "emu.py"), "--headless", "-p", e.profile,
		"-P", strconv.Itoa(e.port), "--mnemonic", trezorMnemonic)
	e.emu.Env = append(os.Environ(), "SDL_VIDEODRIVER=dummy")
	e.emu.Dir = filepath.Join(fw, "core")
	e.emu.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	logf, _ := os.Create(filepath.Join(dir, "trezor-emulator.log"))
	e.emu.Stdout, e.emu.Stderr = logf, logf
	if err := e.emu.Start(); err != nil {
		t.Fatalf("starting the Trezor emulator: %v", err)
	}
	t.Cleanup(e.stop)
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if e.ping() {
			time.Sleep(2 * time.Second) // let emu.py finish loading the seed
			return e
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatal("the Trezor emulator did not come up")
	return nil
}

func (e *emulator) addr() string { return fmt.Sprintf("127.0.0.1:%d", e.port) }

func (e *emulator) ping() bool {
	c, err := net.Dial("udp", e.addr())
	if err != nil {
		return false
	}
	defer c.Close()
	_, _ = c.Write([]byte("PINGPING"))
	_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 16)
	n, err := c.Read(buf)
	return err == nil && strings.HasPrefix(string(buf[:n]), "PONG")
}

// press starts the helper that taps confirm ("yes") or reject ("no") on the
// emulator screen, and logs every screen it sees.
func (e *emulator) press(mode string) {
	e.stopAuto()
	e.autoMode = mode
	e.autoLog = filepath.Join(filepath.Dir(e.profile), "autoconfirm-"+mode+".log")
	logf, _ := os.Create(e.autoLog)
	e.auto = exec.Command(e.python, filepath.Join(repoRoot(e.t), "btc", "e2e", "trezor", "autoconfirm.py"), mode, e.addr())
	e.auto.Stdout, e.auto.Stderr = logf, logf
	e.auto.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := e.auto.Start(); err != nil {
		e.t.Fatal(err)
	}
	time.Sleep(time.Second)
}

func (e *emulator) stopAuto() {
	if e.auto != nil && e.auto.Process != nil {
		_ = syscall.Kill(-e.auto.Process.Pid, syscall.SIGKILL)
		_ = e.auto.Wait()
		e.auto = nil
	}
}

func (e *emulator) screens() string {
	b, _ := os.ReadFile(e.autoLog)
	return string(b)
}

func (e *emulator) stop() {
	e.stopAuto()
	if e.emu != nil && e.emu.Process != nil {
		if raw, err := os.ReadFile(filepath.Join(e.profile, "trezor.pid")); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGTERM)
			}
		}
		_ = syscall.Kill(-e.emu.Process.Pid, syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = e.emu.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-e.emu.Process.Pid, syscall.SIGKILL)
		}
		e.emu = nil
	}
}

var spaces = regexp.MustCompile(`[\s|]+`)

func squash(s string) string { return spaces.ReplaceAllString(s, "") }

func (e *Env) trezorScenarios(t *testing.T, emu *emulator) {
	const name = "hw-trezor"
	udp := "JARVIS_TREZOR_UDP=" + emu.addr()
	ext := func(i uint32) string { return hdAddr(t, trezorMnemonic, 0, i) }
	chg := func(i uint32) string { return hdAddr(t, trezorMnemonic, 1, i) }

	e.scenario("T01-trezor-add", "Adding a Trezor reads the account key, shows the first address on the device, and derives the same addresses as an independent BIP-84 implementation",
		[]string{"Trezor GetPublicKey", "address verification on device", "xpub derivation on host", "no private data stored"}, func(s *S) {
			s.env = []string{udp}
			emu.press("yes")
			out, code := s.jarvis("", "btc", "wallet", "add", "--kind", "trezor", "--desc", name)
			s.expectOK(out, code)
			s.expectContains(out, ext(0))
			if !strings.Contains(squash(emu.screens()), ext(0)) {
				s.fail("the device screen never showed the first address %s:\n%s", ext(0), emu.screens())
			}
			s.note("device screens seen while adding:\n%s", emu.screens())
			rec, err := os.ReadFile(filepath.Join(e.home, ".jarvis", "btc", name+".json"))
			if err != nil {
				s.fail("no record: %v", err)
				return
			}
			if !strings.Contains(string(rec), "tpub") && !strings.Contains(string(rec), "xpub") {
				s.fail("record has no extended public key: %s", rec)
			}
			for _, bad := range []string{"xprv", "tprv", "mnemonic", "abandon"} {
				if strings.Contains(string(rec), bad) {
					s.fail("record contains %q", bad)
				}
			}
			out, code = s.jarvis("", "btc", "wallet", "list")
			s.expectOK(out, code)
			s.expectContains(out, "m/84'/1'/0'", ext(0))
		})

	e.scenario("T02-trezor-balance-unplugged", "HD balance scans receive addresses; works without the device attached",
		[]string{"gap-limit scan", "pending vs confirmed", "offline balance"}, func(s *S) {
			e.fund(ext(0), 0.2, true)
			e.fund(ext(3), 0.3, true)
			e.fund(ext(7), 0.1, false)
			// Balance needs no device: leave JARVIS_TREZOR_UDP pointing at a dead port.
			s.env = []string{"JARVIS_TREZOR_UDP=127.0.0.1:9"}
			out, code := s.jarvis("", "btc", "balance", name)
			s.expectOK(out, code)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.50000000") {
				s.fail("confirmed = %q, want 0.50000000", got)
			}
			if got := field(out, "Pending"); !strings.HasPrefix(got, "0.10000000") {
				s.fail("pending = %q, want 0.10000000", got)
			}
			if got := field(out, "Coins"); got != "3" {
				s.fail("coins = %q, want 3", got)
			}
			e.mine(1)
		})

	e.scenario("T03-trezor-send-multi-input", "Send 0.4 BTC from three funded addresses: the device signs every input with its own path; bitcoind validates the result",
		[]string{"SignTx protocol", "per-input derivation paths", "change on m/84'/1'/0'/1/0", "device shows destination and amount"}, func(s *S) {
			s.env = []string{udp}
			emu.press("yes")
			dest := minerDest(e)
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", dest, "--amount", "0.4", "--fee-rate", "5", "--yes")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid == "" {
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
					s.fail("unexpected output %s (want destination or change %s)", o.ScriptPubKey.Address, chg(0))
				}
			}
			if toDest != 40_000_000 {
				s.fail("destination got %d sat", toDest)
			}
			if toChange == 0 {
				s.fail("no change on the expected change address %s", chg(0))
			}
			me := e.entry(txid)
			s.note("txid %s, %d inputs, vsize %d, fee %d sat, change %d sat at %s", txid, len(d.Vin), me.VSize, satOf(me.Fees.Base), toChange, chg(0))
			screens := squash(emu.screens())
			if !strings.Contains(screens, dest) {
				s.fail("the device never displayed the destination %s", dest)
			}
			if !strings.Contains(screens, "0.4") {
				s.fail("the device never displayed the amount 0.4")
			}
			if feeOnDevice := strings.TrimRight(btcStr(satOf(me.Fees.Base)), "0"); !strings.Contains(screens, "fee:"+feeOnDevice) {
				s.fail("the device did not show the fee %s that bitcoind recorded", feeOnDevice)
			}
			s.note("device screens seen while signing:\n%s", emu.screens())
			e.mine(1)
		})

	e.scenario("T04-trezor-change-advances", "Second payment sends change to the next unused change address",
		[]string{"change index selection", "pending vs used addresses"}, func(s *S) {
			s.env = []string{udp}
			emu.press("yes")
			dest := minerDest(e)
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", dest, "--amount", "0.05", "--fee-rate", "5", "--yes")
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
					s.fail("change reused the already-used change address %s", chg(0))
				}
			}
			if !found {
				s.fail("no change at the next change address %s", chg(1))
			}
			e.mine(1)
		})

	e.scenario("T05-trezor-reject", "Rejecting on the device aborts the send; nothing is broadcast",
		[]string{"user reject on device"}, func(s *S) {
			s.env = []string{udp}
			emu.press("no")
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", minerDest(e), "--amount", "0.01", "--fee-rate", "5", "--yes")
			s.expectFail(out, code, "cancelled", "nothing was broadcast")
			s.expectNotContains(out, "Broadcast ")
			s.expectMempool(0)
			if !strings.Contains(emu.screens(), "-> no") {
				s.fail("the reject helper never saw a confirmation screen")
			}
		})

	e.scenario("T06-trezor-sweep", "Sweeping the whole wallet signs every input on its own path",
		[]string{"sweep with many inputs", "all branches"}, func(s *S) {
			s.env = []string{udp}
			e.fund(ext(12), 0.05, true)
			emu.press("yes")
			dest := minerDest(e)
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", dest, "--amount", "ALL", "--fee-rate", "3", "--yes")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid != "" {
				d := e.decode(txid)
				if len(d.Vout) != 1 || d.Vout[0].ScriptPubKey.Address != dest {
					s.fail("sweep must have exactly one output to the destination")
				}
				s.note("sweep spent %d inputs", len(d.Vin))
				if len(d.Vin) < 3 {
					s.fail("sweep spent only %d inputs", len(d.Vin))
				}
			}
			e.mine(1)
			out, _ = s.jarvis("", "btc", "balance", name)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.00000000") {
				s.fail("wallet not empty after sweep: %q", got)
			}
		})

	e.scenario("T07-trezor-unplugged-send", "Sending with no device attached fails cleanly before anything is built on-chain",
		[]string{"device missing"}, func(s *S) {
			e.fund(ext(0), 0.01, true)
			emu.stop()
			s.env = []string{udp, "JARVIS_BTC_DEVICE_WAIT=2s"}
			out, code := s.jarvis("", "btc", "send", "--from", name, "--to", minerDest(e), "--amount", "0.001", "--fee-rate", "5", "--yes")
			s.expectFail(out, code)
			s.expectNotContains(out, "Broadcast")
			s.expectMempool(0)
		})
}

func minerDest(e *Env) string { return e.walletAddr("miner") }

var _ = btcutil.Hash160
