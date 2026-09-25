package util_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
)

const (
	meHex     = "0x9642b23Ed1E01Df1092B92641051881a322F5D4E"
	routerHex = "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"
	pairHex   = "0x0d4a11d5EEaaC28EC3F61d100daF4d40471f1852"
	usdcHex   = "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	wethHex   = "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"
	hashHex   = "0x3f9a1c2b4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e1c2"
)

func addr(hex, desc string) jarviscommon.Address {
	return jarviscommon.Address{Address: hex, Desc: desc}
}

func addrValue(hex, desc string) jarviscommon.Value {
	a := addr(hex, desc)
	return jarviscommon.Value{Raw: hex, Kind: jarviscommon.DisplayAddress, Address: &a}
}

func tokenValue(raw, symbol string, decimals uint64) jarviscommon.Value {
	return jarviscommon.Value{
		Raw:   raw,
		Kind:  jarviscommon.DisplayToken,
		Token: &jarviscommon.TokenHint{Symbol: symbol, Decimal: decimals},
	}
}

func intValue(raw string) jarviscommon.Value {
	return jarviscommon.Value{Raw: raw, Kind: jarviscommon.DisplayInteger}
}

func scalar(name, typ string, v jarviscommon.Value) jarviscommon.ParamResult {
	return jarviscommon.ParamResult{Name: name, Type: typ, Values: []jarviscommon.Value{v}}
}

func transferLog(token, symbol string, decimals uint64, from, to jarviscommon.Address, raw string) jarviscommon.LogResult {
	return jarviscommon.LogResult{
		Name:    "Transfer",
		Address: addr(token, symbol+" token"),
		Topics: []jarviscommon.TopicResult{
			{Name: "from", Value: addrValue(from.Address, from.Desc)},
			{Name: "to", Value: addrValue(to.Address, to.Desc)},
		},
		Data: []jarviscommon.ParamResult{scalar("value", "uint256", tokenValue(raw, symbol, decimals))},
	}
}

func swapTxResult() *jarviscommon.TxResult {
	me := addr(meHex, "me")
	pair := addr(pairHex, "USDC/WETH pair")
	r := jarviscommon.NewTxResult()
	r.Status = "done"
	r.From = me
	r.To = addr(routerHex, "Uniswap V2 Router")
	r.Value = "0"
	r.Nonce = "412"
	r.GasPrice = "12.5000"
	r.GasLimit = "185123"
	r.GasUsed = "171203"
	r.GasCost = "0.00213000"
	r.BlockNumber = "19234567"
	r.TxType = "contract call"
	r.FunctionCall = &jarviscommon.FunctionCall{
		Destination: r.To,
		Method:      "swapExactTokensForTokens",
		Params: []jarviscommon.ParamResult{
			scalar("amountIn", "uint256", intValue("1000000000")),
			scalar("amountOutMin", "uint256", intValue("311200000000000000")),
			{Name: "path", Type: "address[]", Values: []jarviscommon.Value{
				addrValue(usdcHex, "USDC"), addrValue(wethHex, "WETH"),
			}},
			scalar("to", "address", addrValue(meHex, "me")),
			scalar("deadline", "uint256", intValue("1725600000")),
		},
	}
	r.Logs = []jarviscommon.LogResult{
		transferLog(usdcHex, "USDC", 6, me, pair, "1000000000"),
		{Name: "Sync", Address: pair, Data: []jarviscommon.ParamResult{
			scalar("reserve0", "uint112", intValue("5000000000000")),
			scalar("reserve1", "uint112", intValue("1500000000000000000000")),
		}},
		transferLog(wethHex, "WETH", 18, pair, me, "312100000000000000"),
	}
	return r
}

func render(t *testing.T, r *jarviscommon.TxResult, layout util.TxLayout, hash string) string {
	t.Helper()
	var buf bytes.Buffer
	u := ui.NewTerminalUIWithWriter(&buf, false)
	util.DisplayTxResult(u, r, networks.EthereumMainnet, layout, hash)
	return buf.String()
}

func TestInfoLayoutMaskNamesHidesResolvedNames(t *testing.T) {
	prev := config.MaskNames
	config.MaskNames = true
	t.Cleanup(func() { config.MaskNames = prev })

	out := render(t, swapTxResult(), util.LayoutInfo, hashHex)
	if strings.Contains(out, "me (") || strings.Contains(out, "Uniswap V2 Router") {
		t.Fatalf("resolved names leaked:\n%s", out)
	}
	if !strings.Contains(out, "•••") {
		t.Fatalf("expected masked name:\n%s", out)
	}
}

func TestFullLayoutKeepsFullAddressesAndEventTable(t *testing.T) {
	out := render(t, swapTxResult(), util.LayoutInfoFull, hashHex)

	for _, w := range []string{
		"Hash       " + hashHex,
		"Status     ✓ done",
		"Gas price  12.5000 gwei",
		"Block      19234567",
		routerHex + " (Uniswap V2 Router)",
		"Event Logs",
		"│",
	} {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in full output:\n%s", w, out)
		}
	}
	if strings.Contains(out, "0x7a25…488D") {
		t.Fatalf("full layout must not shorten addresses:\n%s", out)
	}
}

func Test7702DelegationAndSetCode(t *testing.T) {
	r := jarviscommon.NewTxResult()
	r.Status = "done"
	r.TxType = "normal"
	r.From = addr(meHex, "me")
	r.To = addr(meHex, "me")
	r.Value = "0"
	r.GasCost = "0.00031500"
	r.Nonce = "8"
	r.Delegation = addr(routerHex, "BatchCaller")
	r.Authorizations = []jarviscommon.TxAuthorization{{
		Authority: addr(meHex, "me"),
		Address:   addr(routerHex, "BatchCaller"),
		ChainID:   "1",
		Nonce:     "8",
	}}

	out := render(t, r, util.LayoutInfo, hashHex)
	if !strings.Contains(out, "set-code") {
		t.Fatalf("type-4 empty-value tx should read as set-code:\n%s", out)
	}
	if !strings.Contains(out, "delegates to") || !strings.Contains(out, "7702 auth") {
		t.Fatalf("details should name the 7702 dest and auth:\n%s", out)
	}

	full := render(t, r, util.LayoutInfoFull, hashHex)
	if !strings.Contains(full, "Delegates") || !strings.Contains(full, "7702 auth") {
		t.Fatalf("full card missing 7702 rows:\n%s", full)
	}

	r.Authorizations[0].Revoke = true
	r.Authorizations[0].Address = addr("0x0000000000000000000000000000000000000000", "")
	revoke := render(t, r, util.LayoutInfoFull, hashHex)
	if !strings.Contains(revoke, "revokes delegation") {
		t.Fatalf("revoke row missing:\n%s", revoke)
	}
}

func TestZeroAddressNeverShowsAddressBookName(t *testing.T) {
	r := swapTxResult()
	zero := "0x0000000000000000000000000000000000000000"
	r.FunctionCall.Params = []jarviscommon.ParamResult{
		scalar("approveTarget", "address", addrValue(zero, "Quang Le")),
	}
	r.Logs = nil
	for _, layout := range []util.TxLayout{util.LayoutInfo, util.LayoutInfoFull} {
		out := render(t, r, layout, "")
		if strings.Contains(out, "Quang Le") {
			t.Fatalf("zero address must not show an address-book name in layout %v:\n%s", layout, out)
		}
		if !strings.Contains(out, "zero address") {
			t.Fatalf("zero address must be labelled in layout %v:\n%s", layout, out)
		}
	}
}

func TestUnlimitedApprovalIsFlagged(t *testing.T) {
	r := swapTxResult()
	r.FunctionCall = nil
	r.Logs = []jarviscommon.LogResult{{
		Name:    "Approval",
		Address: addr(usdcHex, "USDC token"),
		Topics: []jarviscommon.TopicResult{
			{Name: "owner", Value: addrValue(meHex, "me")},
			{Name: "spender", Value: addrValue(routerHex, "")},
		},
		Data: []jarviscommon.ParamResult{scalar("value", "uint256", tokenValue(
			"115792089237316195423570985008687907853269984665640564039457584007913129639935", "USDC", 6,
		))},
	}}

	rec := ui.NewRecordingUI()
	d := util.DisplayTxResult(rec, r, networks.EthereumMainnet, util.LayoutInfo, "")
	if len(d.Transfers) != 1 || d.Transfers[0].Kind != "approval" || !d.Transfers[0].Unlimited {
		t.Fatalf("expected one unlimited approval, got %+v", d.Transfers)
	}
	if !rec.HasMessage("Approval") || !rec.HasMessage("spender") {
		t.Fatalf("expected the Approval event, got %v", rec.Entries())
	}
}

func TestTxDisplayJSONIsAdditiveAndPlain(t *testing.T) {
	rec := ui.NewRecordingUI()
	d := util.DisplayTxResult(rec, swapTxResult(), networks.EthereumMainnet, util.LayoutInfo, hashHex)
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"hash", "status", "from", "to", "value", "tx_type", "function_call", "logs", "transfers", "block_number", "gas_cost"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing json key %q in %s", k, raw)
		}
	}
	if m["from"] != meHex+" (me)" {
		t.Fatalf("json addresses must stay full: %v", m["from"])
	}
	transfers := m["transfers"].([]any)
	first := transfers[0].(map[string]any)
	if first["token"] != "USDC" || first["amount"] != "1000" {
		t.Fatalf("unexpected transfer json: %v", first)
	}
}

func TestTxDisplayJSONMaskNames(t *testing.T) {
	prev := config.MaskNames
	config.MaskNames = true
	t.Cleanup(func() { config.MaskNames = prev })

	d := util.DisplayTxResult(ui.NewRecordingUI(), swapTxResult(), networks.EthereumMainnet, util.LayoutInfo, hashHex)
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["from"] != meHex+" (•••)" {
		t.Fatalf("json must mask names, got %v", m["from"])
	}
	if m["to"] != routerHex+" (•••)" {
		t.Fatalf("json must mask names, got %v", m["to"])
	}
}

func TestUndecodedEventsAreCountedAndShownRaw(t *testing.T) {
	r := swapTxResult()
	r.Logs = append(r.Logs, jarviscommon.LogResult{
		Address: addr(pairHex, ""),
		Topics: []jarviscommon.TopicResult{
			{Name: "topic0", Value: jarviscommon.Value{Raw: "0x" + strings.Repeat("ab", 32), Kind: jarviscommon.DisplayRaw}},
		},
		Data: []jarviscommon.ParamResult{{Name: "data", Type: "bytes", Values: []jarviscommon.Value{{Raw: "0x" + strings.Repeat("00", 64), Kind: jarviscommon.DisplayRaw}}}},
	})
	out := render(t, r, util.LayoutInfo, hashHex)
	if !strings.Contains(out, "Events (4)") {
		t.Fatalf("undecoded logs must count:\n%s", out)
	}
	if !strings.Contains(out, "4. <undecoded>  0x0d4a…1852   topic0 0xabab") {
		t.Fatalf("raw event line missing:\n%s", out)
	}
	if !strings.Contains(out, "1 event(s) shown raw") {
		t.Fatalf("raw-event note missing:\n%s", out)
	}
}

func TestTransfersUseTopicPositionsForDaiStyleNames(t *testing.T) {
	r := swapTxResult()
	r.FunctionCall = nil
	me := addr(meHex, "me")
	router := addr(routerHex, "Uniswap V2 Router")
	r.Logs = []jarviscommon.LogResult{{
		Name:    "Approval",
		Address: addr(wethHex, "WETH token"),
		Topics: []jarviscommon.TopicResult{
			{Name: "src", Value: addrValue(me.Address, me.Desc)},
			{Name: "guy", Value: addrValue(router.Address, router.Desc)},
		},
		Data: []jarviscommon.ParamResult{scalar("wad", "uint256", tokenValue("5000000000000000000", "WETH", 18))},
	}}
	rec := ui.NewRecordingUI()
	d := util.DisplayTxResult(rec, r, networks.EthereumMainnet, util.LayoutInfo, hashHex)
	if len(d.Transfers) != 1 || d.Transfers[0].Kind != "approval" {
		t.Fatalf("expected one approval transfer, got %+v", d.Transfers)
	}
	if !strings.Contains(d.Transfers[0].From.Text, meHex) || !strings.Contains(d.Transfers[0].To.Text, routerHex) {
		t.Fatalf("dai-style src/guy topics should fill from/to: %+v", d.Transfers[0])
	}
	if rec.HasMessage("Transfers") {
		t.Fatal("must not print a Transfers subsection")
	}
	if !rec.HasMessage("Approval") || !rec.HasMessage("guy") {
		t.Fatalf("events should still show the dai-style approval, got %v", rec.Entries())
	}
}

func TestTimestampParamsShowTheDate(t *testing.T) {
	r := swapTxResult()
	compact := render(t, r, util.LayoutInfo, hashHex)
	if !strings.Contains(compact, "  deadline      2024-09-06 05:20:00 UTC, ") || strings.Contains(compact, "1725600000") {
		t.Fatalf("compact layout should show the deadline as a date only:\n%s", compact)
	}
	full := render(t, r, util.LayoutInfoFull, hashHex)
	if !strings.Contains(full, "  deadline      1725600000 (2024-09-06 05:20:00 UTC, ") {
		t.Fatalf("full layout should keep the raw seconds next to the date:\n%s", full)
	}
	// The echo of a typed parameter goes through the same path, so an operator
	// who mistypes a deadline sees "2 years ago" before signing.
	echo := util.ParamValueLines(ui.NewRecordingUI(), util.NewParamDisplay(scalar("deadline", "uint256", intValue("1725600000"))))
	if len(echo) != 1 || !strings.HasPrefix(echo[0], "2024-09-06 05:20:00 UTC, ") || !strings.HasSuffix(echo[0], " ago") {
		t.Fatalf("echo should be the date with a relative hint, got %q", echo)
	}
}

func TestNetEffectSummarisesManyTransfers(t *testing.T) {
	me := addr(meHex, "me")
	pair := addr(pairHex, "USDC/WETH pair")
	router := addr(routerHex, "Uniswap V2 Router")
	zero := addr("0x0000000000000000000000000000000000000000", "")
	r := swapTxResult()
	r.FunctionCall = nil
	// me → router → pair → router → me: router is a pass-through and must not
	// appear; a burn to the zero address must not add a row.
	r.Logs = []jarviscommon.LogResult{
		transferLog(usdcHex, "USDC", 6, me, router, "1000000000"),
		transferLog(usdcHex, "USDC", 6, router, pair, "1000000000"),
		transferLog(wethHex, "WETH", 18, pair, router, "312100000000000000"),
		transferLog(wethHex, "WETH", 18, router, me, "312100000000000000"),
		transferLog(usdcHex, "USDC", 6, pair, zero, "5000000"),
	}
	rec := ui.NewRecordingUI()
	d := util.DisplayTxResult(rec, r, networks.EthereumMainnet, util.LayoutInfo, hashHex)
	if len(d.NetEffect) != 2 {
		t.Fatalf("expected 2 net rows (me, pair), got %+v", d.NetEffect)
	}
	if d.NetEffect[0].Address.Text != meHex+" (me)" {
		t.Fatalf("sender must come first: %+v", d.NetEffect[0])
	}
	if got := strings.Join(d.NetEffect[0].Deltas, " | "); got != "-1000 USDC | +0.3121 WETH" {
		t.Fatalf("sender deltas %q", got)
	}
	if got := strings.Join(d.NetEffect[1].Deltas, " | "); got != "+995 USDC | -0.3121 WETH" {
		t.Fatalf("pair deltas %q", got)
	}

	out := render(t, r, util.LayoutInfo, hashHex)
	if !strings.Contains(out, "Net effect") || !strings.Contains(out, "-1,000 USDC") {
		t.Fatalf("net effect block:\n%s", out)
	}
	if strings.Contains(out, "\nTransfers\n") {
		t.Fatalf("net effect txs must not also print a Transfers list:\n%s", out)
	}

	r.Logs = r.Logs[:3]
	d = util.DisplayTxResult(ui.NewRecordingUI(), r, networks.EthereumMainnet, util.LayoutInfo, hashHex)
	if d.NetEffect != nil {
		t.Fatalf("three transfers need no summary: %+v", d.NetEffect)
	}
}

func TestTxDisplayJSONOmitsClearSign(t *testing.T) {
	var buf bytes.Buffer
	u := ui.NewTerminalUIWithWriter(&buf, false)
	d := util.DisplayTxResultWith(u, swapTxResult(), networks.EthereumMainnet, util.LayoutInfo, hashHex,
		func(d *util.TxDisplay, _ *jarviscommon.TxResult) {
			d.ClearSign = func(ui.UI) {}
		})
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "ClearSign") {
		t.Fatalf("JSON must omit the ClearSign callback: %s", raw)
	}
}
