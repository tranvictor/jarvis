//go:build e2e

package e2e

import (
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/tranvictor/jarvis/btc/e2e/shim"
)

const (
	password   = "hunter2"
	testKeyHex = "0000000000000000000000000000000000000000000000000000000000000001"
)

func regtestAddr(pub []byte) string {
	a, _ := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(pub), &chaincfg.RegressionNetParams)
	return a.EncodeAddress()
}

func writeFile(t *testing.T, path, content string) string {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func btcStr(sat int64) string { return fmt.Sprintf("%.8f", float64(sat)/1e8) }

func satOf(btc float64) int64 { return int64(math.Round(btc * 1e8)) }

// newGethKeystore writes a keystore holding key and returns its path.
func newGethKeystore(t *testing.T, dir, keyHex, pass string) string {
	raw, _ := hex.DecodeString(keyHex)
	priv, _ := btcec.PrivKeyFromBytes(raw)
	ks := gethkeystore.NewKeyStore(dir, gethkeystore.LightScryptN, gethkeystore.LightScryptP)
	acc, err := ks.ImportECDSA(priv.ToECDSA(), pass)
	if err != nil {
		t.Fatal(err)
	}
	return acc.URL.Path
}

// field pulls "Label   value" out of a key/value card.
func field(out, label string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(label) + `\s+(.+)$`)
	if m := re.FindStringSubmatch(out); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func TestBitcoinE2E(t *testing.T) {
	e := newEnv(t)
	pwFile := writeFile(t, filepath.Join(e.dir, "pw"), password+"\n")
	badPwFile := writeFile(t, filepath.Join(e.dir, "badpw"), "not-the-password\n")
	minerDest := func() string { return e.walletAddr("miner") }

	t.Cleanup(func() {
		dir := os.Getenv("E2E_ARTIFACT_DIR")
		if dir == "" {
			dir = filepath.Join(repoRoot(t), "btc", "e2e", "artifacts")
		}
		e.writeReport(dir)
	})

	// Addresses derived independently of jarvis, with btcd.
	rawKey, _ := hex.DecodeString(testKeyHex)
	_, pub1 := btcec.PrivKeyFromBytes(rawKey)
	key1Addr := regtestAddr(pub1.SerializeCompressed())
	const key1Wallet = "k1"

	e.scenario("S01-isolation", "Bitcoin wallets never leak into the Ethereum wallet store",
		[]string{"ETH wallet list unchanged", "BTC records live under ~/.jarvis/btc"}, func(s *S) {
			before, c1 := s.jarvis("", "wallet", "list")
			s.note("ethereum `wallet list` before: exit %d", c1)
			out, code := s.jarvis("", "btc", "wallet", "list")
			s.expectOK(out, code)
			s.expectContains(out, "No Bitcoin wallets yet")

			path := writeFile(t, filepath.Join(e.dir, "k1.hex"), testKeyHex+"\n")
			out, code = s.jarvis("", "btc", "wallet", "add", "--kind", "privatekey", "--desc", key1Wallet,
				"--private-key-file", path, "--password-file", pwFile)
			s.expectOK(out, code)
			s.expectContains(out, key1Addr)

			after, _ := s.jarvis("", "wallet", "list")
			if before != after {
				s.fail("ethereum wallet list changed after adding a bitcoin wallet:\n%s\n---\n%s", before, after)
			}
			root := filepath.Join(e.home, ".jarvis")
			matches, _ := filepath.Glob(filepath.Join(root, "*.json"))
			if len(matches) != 0 {
				s.fail("bitcoin wallet leaked into the Ethereum glob ~/.jarvis/*.json: %v", matches)
			}
			if _, err := os.Stat(filepath.Join(root, "btc", key1Wallet+".json")); err != nil {
				s.fail("record not stored under ~/.jarvis/btc: %v", err)
			}
			st, err := os.Stat(filepath.Join(root, "btc", key1Wallet+".json"))
			if err == nil && st.Mode().Perm() != 0o600 {
				s.fail("wallet record mode is %v, want 0600", st.Mode().Perm())
			}
			ksPath := filepath.Join(root, "btc", "keystores", key1Wallet+".json")
			if raw, err := os.ReadFile(ksPath); err != nil {
				s.fail("keystore missing: %v", err)
			} else if strings.Contains(string(raw), testKeyHex) {
				s.fail("private key stored in clear text")
			}
		})

	e.scenario("S02-add-keystore", "Existing Ethereum-style keystore becomes a bc1q wallet (right and wrong password)",
		[]string{"keystore import", "address independently derived", "wrong passphrase leaves no record"}, func(s *S) {
			const key2 = "00000000000000000000000000000000000000000000000000000000000000f2"
			ks := newGethKeystore(t, filepath.Join(e.dir, "ks"), key2, password)
			raw, _ := hex.DecodeString(key2)
			_, pub := btcec.PrivKeyFromBytes(raw)
			want := regtestAddr(pub.SerializeCompressed())

			out, code := s.jarvis("", "btc", "wallet", "add", "--kind", "keystore", "--desc", "bad-pass",
				"--keystore", ks, "--password-file", badPwFile)
			s.expectFail(out, code)
			out, code = s.jarvis("", "btc", "wallet", "list")
			s.expectNotContains(out, "bad-pass")

			out, code = s.jarvis("", "btc", "wallet", "add", "--kind", "keystore", "--desc", "ks-two",
				"--keystore", ks, "--password-file", pwFile)
			s.expectOK(out, code)
			s.expectContains(out, want)
			s.note("expected address %s derived independently with btcd", want)

			out, code = s.jarvis("", "btc", "wallet", "add", "--kind", "keystore", "--desc", "ks-two",
				"--keystore", ks, "--password-file", pwFile)
			s.expectFail(out, code, "already exists")
		})

	e.scenario("S03-add-privatekey-forms", "Private key accepted as hex and WIF; wrong-chain WIF and garbage rejected",
		[]string{"WIF import", "wrong-network key", "invalid key"}, func(s *S) {
			wif, err := btcutil.NewWIF(func() *btcec.PrivateKey { k, _ := btcec.PrivKeyFromBytes(rawKey); return k }(),
				&chaincfg.RegressionNetParams, true)
			if err != nil {
				t.Fatal(err)
			}
			f := writeFile(t, filepath.Join(e.dir, "wif.txt"), wif.String()+"\n")
			out, code := s.jarvis("", "btc", "wallet", "add", "--kind", "privatekey", "--desc", "from-wif",
				"--private-key-file", f, "--password-file", pwFile)
			s.expectOK(out, code)
			s.expectContains(out, key1Addr)

			mainWIF, _ := btcutil.NewWIF(func() *btcec.PrivateKey { k, _ := btcec.PrivKeyFromBytes(rawKey); return k }(),
				&chaincfg.MainNetParams, true)
			f = writeFile(t, filepath.Join(e.dir, "mainwif.txt"), mainWIF.String()+"\n")
			out, code = s.jarvis("", "btc", "wallet", "add", "--kind", "privatekey", "--desc", "wrong-net",
				"--private-key-file", f, "--password-file", pwFile)
			s.expectFail(out, code, "another chain")

			f = writeFile(t, filepath.Join(e.dir, "junk.txt"), "definitely not a key\n")
			out, code = s.jarvis("", "btc", "wallet", "add", "--kind", "privatekey", "--desc", "junk",
				"--private-key-file", f, "--password-file", pwFile)
			s.expectFail(out, code)

			zero := writeFile(t, filepath.Join(e.dir, "zero.txt"), strings.Repeat("0", 64)+"\n")
			out, code = s.jarvis("", "btc", "wallet", "add", "--kind", "privatekey", "--desc", "zero",
				"--private-key-file", zero, "--password-file", pwFile)
			s.expectFail(out, code)

			list, _ := s.jarvis("", "btc", "wallet", "list")
			s.expectNotContains(list, "wrong-net", "junk", "zero")
		})

	e.scenario("S04-fees", "Fee rates come from the node's estimates; an empty table is an error, not a guess",
		[]string{"fee table", "no fee estimate available", "--fee-rate override"}, func(s *S) {
			out, code := s.jarvis("", "btc", "fees")
			s.expectOK(out, code)
			s.expectContains(out, "regtest", "20.00", "10.00", "1.00")
			s.note("shim serves {1:20, 2:10, 3:8, 6:4, 12:2, 24:1.5, 144:1}; the 1-block row shows 20 sat/vB x 141 vB = 2820 sat")
			s.expectContains(out, "2820")

			e.shim.SetFees(nil)
			out, code = s.jarvis("", "btc", "fees")
			s.expectFail(out, code, "fee")
			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", minerDest(), "--amount", "0.001",
				"--password-file", pwFile, "-Y")
			s.expectFail(out, code, "--fee-rate")
			s.expectMempool(0)
			e.shim.SetFees(map[int]float64{1: 20, 2: 10, 3: 8, 6: 4, 12: 2, 24: 1.5, 144: 1})
		})

	e.scenario("S05-balance", "Balances: empty, confirmed, pending; watch-only address; wrong-chain address",
		[]string{"confirmed vs pending vs spendable", "watch-only", "wrong network"}, func(s *S) {
			out, code := s.jarvis("", "btc", "balance", key1Wallet)
			s.expectOK(out, code)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.00000000") {
				s.fail("unfunded wallet confirmed = %q", got)
			}

			e.fund(key1Addr, 0.5, true)
			e.fund(key1Addr, 0.25, true)
			out, code = s.jarvis("", "btc", "balance", key1Wallet)
			s.expectOK(out, code)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.75000000") {
				s.fail("confirmed = %q, want 0.75000000", got)
			}
			if got := field(out, "Coins"); got != "2" {
				s.fail("coins = %q, want 2", got)
			}

			e.fund(key1Addr, 0.1, false)
			out, code = s.jarvis("", "btc", "balance", key1Wallet)
			s.expectOK(out, code)
			if got := field(out, "Pending"); !strings.HasPrefix(got, "0.10000000") {
				s.fail("pending = %q, want 0.10000000", got)
			}
			if got := field(out, "Spendable now"); !strings.HasPrefix(got, "0.75000000") {
				s.fail("spendable = %q, want 0.75000000 (unconfirmed funds are not spendable by default)", got)
			}
			e.mine(1)

			watch := e.walletAddr("miner")
			out, code = s.jarvis("", "btc", "balance", watch)
			s.expectOK(out, code)
			s.expectContains(out, watch)

			out, code = s.jarvis("", "btc", "balance", "bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq")
			s.expectFail(out, code, "network")
			out, code = s.jarvis("", "btc", "balance", "no-such-wallet")
			s.expectFail(out, code)
		})

	// Every send below is validated by bitcoind itself: the shim forwards
	// the signed transaction to sendrawtransaction.
	e.scenario("S06-send-basic", "Send 0.1 BTC: bitcoind accepts it; outputs, change and fee rate are as promised",
		[]string{"signing", "BIP-143 witness validity (bitcoind verified)", "change output", "fee = rate x vsize", "balance after confirmation"}, func(s *S) {
			dest := minerDest()
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.1",
				"--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			s.expectContains(out, "Confirm Bitcoin payment", "10.00 sat/vB")
			if txid == "" {
				return
			}
			d := e.decode(txid)
			me := e.entry(txid)
			var toDest, toChange int64
			for _, o := range d.Vout {
				switch o.ScriptPubKey.Address {
				case dest:
					toDest += satOf(o.Value)
				case key1Addr:
					toChange += satOf(o.Value)
				default:
					s.fail("unexpected output to %s", o.ScriptPubKey.Address)
				}
			}
			if toDest != 10_000_000 {
				s.fail("destination got %d sat, want 10000000", toDest)
			}
			fee := satOf(me.Fees.Base)
			if len(d.Vin) != 1 {
				s.fail("inputs = %d, want 1 (largest-first coin selection)", len(d.Vin))
			}
			rate := float64(fee) / float64(me.VSize)
			if rate < 10.0 || rate > 10.5 {
				s.fail("effective fee rate %.2f sat/vB, want about 10", rate)
			}
			s.note("txid %s, vsize %d, fee %d sat (%.2f sat/vB), change %d sat", txid, me.VSize, fee, rate, toChange)
			if toChange+toDest+fee != 50_000_000 {
				s.fail("value not conserved: %d + %d + %d != 50000000", toChange, toDest, fee)
			}
			e.mine(1)
			out, _ = s.jarvis("", "btc", "balance", key1Wallet)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, btcStr(25_000_000+10_000_000+toChange)) {
				s.fail("confirmed after block = %q, want %s", got, btcStr(25_000_000+10_000_000+toChange))
			}
			if got := field(out, "Pending"); !strings.HasPrefix(got, "0.00000000") {
				s.fail("pending after block = %q", got)
			}
			s.note("balance after confirmation:\n%s", out)
		})

	e.scenario("S07-send-sat-units", "Amounts may be given in satoshi",
		[]string{"unit parsing"}, func(s *S) {
			dest := minerDest()
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "123456 sat",
				"--fee-rate", "3", "--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid == "" {
				return
			}
			var got int64
			for _, o := range e.decode(txid).Vout {
				if o.ScriptPubKey.Address == dest {
					got = satOf(o.Value)
				}
			}
			if got != 123456 {
				s.fail("destination got %d sat, want 123456", got)
			}
			e.mine(1)
		})

	e.scenario("S08-invalid-input", "Bad amounts, bad addresses, wrong network: refused before anything is signed or sent",
		[]string{"amount parsing", "address validation", "dust", "insufficient funds"}, func(s *S) {
			dest := minerDest()
			base := []string{"btc", "send", "--from", key1Wallet, "--password-file", pwFile, "-Y"}
			try := func(to, amount string, contains ...string) {
				s.t.Helper()
				args := append(append([]string{}, base...), "--to", to, "--amount", amount)
				out, code := s.jarvis("", args...)
				s.expectFail(out, code, contains...)
			}
			for _, bad := range []string{"0.123456789", "-1", "abc", "0", "", "21000001", "1e3", "1,5"} {
				try(dest, bad)
			}
			try(dest, "100 sat", "dust")
			try(dest, "1000 BTC", "insufficient")
			try("bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq", "0.01", "network")
			try("tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx", "0.01", "network")
			try("1BvBMSEYstWetqTFn5Au4m4GFg7xJaNVN2", "0.01", "network")
			try("notanaddress", "0.01")
			try(dest[:len(dest)-1]+"x", "0.01")
			try(strings.ToUpper(dest[:8])+dest[8:], "0.01")
			s.expectMempool(0)
			s.note("mempool still empty: nothing was broadcast by any of these attempts")
		})

	e.scenario("S09-fee-guards", "Fee rate sanity: absurd rates refused, high fee share warned about",
		[]string{"max fee rate", "fee below minimum", "fee/amount ratio warning"}, func(s *S) {
			dest := minerDest()
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.01",
				"--fee-rate", "100000", "--password-file", pwFile, "-Y")
			s.expectFail(out, code, "safety limit")
			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.01",
				"--fee-rate", "0.5", "--password-file", pwFile, "-Y")
			s.expectFail(out, code, "1 sat/vB")
			s.expectMempool(0)

			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "2000 sat",
				"--fee-rate", "50", "--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			s.expectContains(out, "% of the amount")
			e.mine(1)
		})

	e.scenario("S10-no-broadcast", "--no-broadcast prints a valid signed transaction and sends nothing",
		[]string{"offline signing", "hex validity (testmempoolaccept)"}, func(s *S) {
			dest := minerDest()
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.02",
				"--password-file", pwFile, "-Y", "--no-broadcast")
			s.expectOK(out, code)
			s.expectMempool(0)
			m := regexp.MustCompile(`(?m)^([0-9a-f]{200,})$`).FindStringSubmatch(out)
			if m == nil {
				s.fail("no raw transaction hex in output:\n%s", out)
				return
			}
			var res []struct {
				Allowed bool   `json:"allowed"`
				Reject  string `json:"reject-reason"`
			}
			if err := e.rpc.CallInto(&res, "testmempoolaccept", []string{m[1]}); err != nil {
				s.fail("testmempoolaccept: %v", err)
				return
			}
			if len(res) != 1 || !res[0].Allowed {
				s.fail("bitcoind rejects the printed transaction: %+v", res)
			}
			s.note("bitcoind testmempoolaccept: allowed=%v", len(res) == 1 && res[0].Allowed)
			var txid string
			if err := e.rpc.CallInto(&txid, "sendrawtransaction", m[1]); err != nil {
				s.fail("broadcasting the printed hex by hand: %v", err)
			}
			e.mine(1)
		})

	e.scenario("S11-decline", "Answering no at the confirmation signs and sends nothing",
		[]string{"user decline"}, func(s *S) {
			dest := minerDest()
			out, code := s.jarvis("n\n", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.02",
				"--password-file", pwFile)
			s.expectOK(out, code)
			s.expectContains(out, "Confirm Bitcoin payment")
			s.expectNotContains(out, "Broadcast")
			s.expectMempool(0)
		})

	e.scenario("S12-wrong-password", "A wrong keystore password stops the send before broadcast",
		[]string{"bad password"}, func(s *S) {
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", minerDest(), "--amount", "0.02",
				"--password-file", badPwFile, "-Y")
			s.expectFail(out, code)
			s.expectNotContains(out, "Broadcast")
			s.expectMempool(0)
		})

	e.scenario("S13-wrong-key-in-keystore", "A keystore swapped for another key is detected and nothing is signed",
		[]string{"wallet/key mismatch"}, func(s *S) {
			const other = "00000000000000000000000000000000000000000000000000000000000000a7"
			ksFile := filepath.Join(e.home, ".jarvis", "btc", "keystores", key1Wallet+".json")
			orig, err := os.ReadFile(ksFile)
			if err != nil {
				t.Fatal(err)
			}
			swapped := newGethKeystore(t, filepath.Join(e.dir, "swap"), other, password)
			raw, _ := os.ReadFile(swapped)
			if err := os.WriteFile(ksFile, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			defer os.WriteFile(ksFile, orig, 0o600)
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", minerDest(), "--amount", "0.02",
				"--password-file", pwFile, "-Y")
			s.expectFail(out, code, "different")
			s.expectMempool(0)
		})

	e.scenario("S14-unconfirmed-chain", "Unconfirmed change is not spent unless asked; with --spend-unconfirmed the chain is valid",
		[]string{"confirmed-only default", "--spend-unconfirmed", "chained transaction validity"}, func(s *S) {
			// Leave the wallet with one small confirmed coin and a large pending change.
			sweep := minerDest()
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", sweep, "--amount", "ALL",
				"--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			e.mine(1)
			funder := key1Addr
			e.fund(funder, 0.01, true)
			first := e.fund(funder, 0.5, false) // pending
			s.note("pending funding %s", first)

			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", sweep, "--amount", "0.3",
				"--password-file", pwFile, "-Y")
			s.expectFail(out, code, "unconfirmed")
			s.expectMempool(1)

			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", sweep, "--amount", "0.3",
				"--password-file", pwFile, "-Y", "--spend-unconfirmed")
			s.expectOK(out, code)
			s.expectContains(out, "unconfirmed")
			s.expectMempool(2)
			e.mine(1)
			s.expectMempool(0)
		})

	e.scenario("S15-dust-change-folded", "Change below the dust limit is added to the fee, not created",
		[]string{"dust change", "sweep (ALL)"}, func(s *S) {
			// Start from a clean wallet: sweep what is left, then fund exactly 100000 sat.
			dest := minerDest()
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "ALL",
				"--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			e.mine(1)
			e.fund(key1Addr, 0.001, true)

			// 1 input, 1 output at 2 sat/vB is 110 vB = 220 sat. Ask for coin - 300 sat so the
			// change would be 80 sat (below dust) and must be folded into the fee.
			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "99700 sat",
				"--fee-rate", "2", "--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			txid := s.broadcastTxid(out)
			if txid == "" {
				return
			}
			d := e.decode(txid)
			if len(d.Vout) != 1 {
				s.fail("expected a single output (no dust change), got %d", len(d.Vout))
			}
			fee := satOf(e.entry(txid).Fees.Base)
			if fee != 300 {
				s.fail("fee = %d sat, want 300 (220 for size + 80 folded dust)", fee)
			}
			e.mine(1)

			e.fund(key1Addr, 0.002, true)
			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "ALL",
				"--fee-rate", "5", "--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			txid = s.broadcastTxid(out)
			if txid != "" {
				d := e.decode(txid)
				if len(d.Vout) != 1 {
					s.fail("sweep must have exactly one output, got %d", len(d.Vout))
				}
				me := e.entry(txid)
				if satOf(d.Vout[0].Value)+satOf(me.Fees.Base) != 200_000 {
					s.fail("sweep does not conserve value")
				}
				s.note("sweep: sent %d sat, fee %d sat, vsize %d", satOf(d.Vout[0].Value), satOf(me.Fees.Base), me.VSize)
			}
			e.mine(1)
			out, _ = s.jarvis("", "btc", "balance", key1Wallet)
			if got := field(out, "Confirmed"); !strings.HasPrefix(got, "0.00000000") {
				s.fail("wallet not empty after sweep: %s", got)
			}
		})

	e.scenario("S16-indexer-failures", "Esplora outage and garbage responses are reported, never guessed around",
		[]string{"HTTP 500", "invalid JSON", "no broadcast"}, func(s *S) {
			e.fund(key1Addr, 0.01, true)
			dest := minerDest()
			for name, mode := range map[string]shim.Mode{"HTTP 500": shim.ServerError, "bad JSON": shim.BadJSON} {
				e.shim.SetMode(mode)
				out, code := s.jarvis("", "btc", "balance", key1Wallet)
				if code == 0 {
					s.fail("%s: balance succeeded:\n%s", name, out)
				}
				out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.001",
					"--fee-rate", "5", "--password-file", pwFile, "-Y")
				if code == 0 {
					s.fail("%s: send succeeded:\n%s", name, out)
				}
				s.expectNotContains(out, "Broadcast")
			}
			e.shim.SetMode(shim.Normal)
			s.expectMempool(0)
			out, code := s.jarvis("", "btc", "balance", key1Wallet)
			s.expectOK(out, code)
		})

	e.scenario("S17-stale-index-double-spend", "Spending a coin the node already spent: the node's rejection is surfaced, nothing is reported as sent",
		[]string{"stale UTXO list", "broadcast rejected", "no false success"}, func(s *S) {
			dest := minerDest()
			if err := e.shim.Freeze(key1Addr); err != nil {
				t.Fatal(err)
			}
			// Spend everything once, for real.
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "ALL",
				"--password-file", pwFile, "-Y")
			s.expectOK(out, code)
			e.mine(1)
			// The indexer now lags: it still lists the spent coins.
			e.shim.SetMode(shim.StaleUTXOs)
			out, code = s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.001",
				"--fee-rate", "5", "--password-file", pwFile, "-Y")
			e.shim.SetMode(shim.Normal)
			s.expectFail(out, code, "broadcast failed", "missingorspent")
			s.expectNotContains(out, "Broadcast ")
			s.expectMempool(0)
		})

	e.scenario("S18-fee-rate-from-target", "--target picks the estimate for that confirmation target",
		[]string{"fee target selection"}, func(s *S) {
			e.fund(key1Addr, 0.01, true)
			dest := minerDest()
			for target, want := range map[int]float64{1: 20, 6: 4, 144: 1} {
				out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.001",
					"--target", strconv.Itoa(target), "--password-file", pwFile, "-Y", "--no-broadcast")
				s.expectOK(out, code)
				s.expectContains(out, fmt.Sprintf("%.2f sat/vB", want))
			}
			s.expectMempool(0)
		})

	e.scenario("S19-destination-types", "Every standard destination type is payable; dust limits follow the type",
		[]string{"P2PKH", "P2SH", "P2WPKH", "P2WSH-sized program", "P2TR", "per-type dust threshold"}, func(s *S) {
			e.fund(key1Addr, 0.05, true)
			rpc := e.walletRPC("miner")
			types := map[string]string{"legacy": "P2PKH", "p2sh-segwit": "P2SH", "bech32": "P2WPKH", "bech32m": "P2TR"}
			for kind, label := range types {
				var dest string
				if err := rpc.CallInto(&dest, "getnewaddress", "", kind); err != nil {
					s.fail("getnewaddress %s: %v", kind, err)
					continue
				}
				out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", dest, "--amount", "0.001",
					"--fee-rate", "2", "--password-file", pwFile, "-Y", "--spend-unconfirmed")
				s.expectOK(out, code)
				txid := s.broadcastTxid(out)
				if txid == "" {
					continue
				}
				found := false
				for _, o := range e.decode(txid).Vout {
					if o.ScriptPubKey.Address == dest && satOf(o.Value) == 100_000 {
						found = true
					}
				}
				if !found {
					s.fail("%s: output to %s not found in %s", label, dest, txid)
				}
				s.note("%s destination %s accepted by bitcoind in %s", label, dest, txid)
			}
			var legacy string
			_ = rpc.CallInto(&legacy, "getnewaddress", "", "legacy")
			out, code := s.jarvis("", "btc", "send", "--from", key1Wallet, "--to", legacy, "--amount", "500 sat",
				"--fee-rate", "2", "--password-file", pwFile, "-Y", "--spend-unconfirmed")
			s.expectFail(out, code, "dust")
			e.mine(1)
		})

	if dev := startSpeculos(t, e.dir, ledgerMnemonic, "main"); dev != nil {
		wrong := startSpeculos(t, e.dir, "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong", "wrong")
		e.ledgerScenarios(t, dev, wrong)
	} else if os.Getenv("E2E_REQUIRE_LEDGER") == "1" {
		t.Fatal("E2E_REQUIRE_LEDGER=1 but SPECULOS_BIN / LEDGER_APP_ELF are not set")
	} else {
		e.scenario("L00-ledger-speculos", "Ledger scenarios (HD wallet, sign_psbt, reject, wrong device)",
			[]string{"needs the Ledger Bitcoin Test app running in Speculos"}, func(s *S) {
				s.note("NOT RUN: set SPECULOS_BIN and LEDGER_APP_ELF (see scripts/e2e-btc.sh)")
				s.t.Skip("no Speculos configured")
			})
	}

	if emu := startEmulator(t, e.dir); emu != nil {
		e.trezorScenarios(t, emu)
	} else if os.Getenv("E2E_REQUIRE_TREZOR") == "1" {
		t.Fatal("E2E_REQUIRE_TREZOR=1 but TREZOR_FIRMWARE_DIR / TREZOR_PYTHON are not set")
	} else {
		e.scenario("T00-trezor-emulator", "Trezor scenarios (HD wallet, SignTx, reject, change path)",
			[]string{"needs a Trezor firmware emulator"}, func(s *S) {
				s.note("NOT RUN: set TREZOR_FIRMWARE_DIR (a trezor-firmware checkout with the emulator built) and TREZOR_PYTHON (its venv python)")
				s.t.Skip("no Trezor emulator configured")
			})
	}
}
