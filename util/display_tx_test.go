package util_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
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

func TestInfoLayoutOrdersByImportance(t *testing.T) {
	out := render(t, swapTxResult(), util.LayoutInfo, hashHex)

	want := []string{
		"✓ done   swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)",
		"         from me (0x9642…5D4E)   value 0 ETH   gas 0.00213000 ETH   nonce 412   block 19234567",
		"Transfers",
		"  1000 USDC     me (0x9642…5D4E)  →  USDC/WETH pair (0x0d4a…1852)",
		"  0.3121 WETH   USDC/WETH pair (0x0d4a…1852)  →  me (0x9642…5D4E)",
		"Call  swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)",
		"  amountIn      1000000000 (1‸000￺000￺000)  uint256",
		"  path          [2 items]  address[]",
		"  ├─ USDC (0xA0b8…eB48)",
		"  └─ WETH (0xC02a…6Cc2)",
		"  to            me (0x9642…5D4E)  address",
		"Events (3)",
		"  1. Transfer  USDC token (0xA0b8…eB48)   from me (0x9642…5D4E)  to USDC/WETH pair (0x0d4a…1852)  value 1000000000 (1000 USDC)",
		"  2. Sync      USDC/WETH pair (0x0d4a…1852)   reserve0 5000000000000",
		"✓ done   swapExactTokensForTokens  →  Uniswap V2 Router (0x7a25…488D)   0x3f9a…e1c2",
	}
	pos := -1
	for _, w := range want {
		idx := strings.Index(out, w)
		if idx < 0 {
			t.Fatalf("missing %q in output:\n%s", w, out)
		}
		if idx < pos {
			t.Fatalf("%q appears out of order in output:\n%s", w, out)
		}
		pos = idx
	}
	if strings.Contains(out, "│") || strings.Contains(out, "╭") {
		t.Fatalf("compact layout must not draw table borders:\n%s", out)
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

func TestInfoLayoutCollapsesLongArraysAndHex(t *testing.T) {
	r := swapTxResult()
	var many []jarviscommon.Value
	for i := 0; i < 6; i++ {
		many = append(many, intValue("1"))
	}
	r.FunctionCall.Params = []jarviscommon.ParamResult{
		{Name: "ids", Type: "uint256[]", Values: many},
		scalar("data", "bytes", jarviscommon.Value{Raw: "0x" + strings.Repeat("ab", 100), Kind: jarviscommon.DisplayRaw}),
	}
	r.Logs = nil

	out := render(t, r, util.LayoutInfo, "")
	if !strings.Contains(out, "ids   [6 items]  (-x to expand)  uint256[]") {
		t.Fatalf("expected collapsed array:\n%s", out)
	}
	if !strings.Contains(out, "data  0xabab…abab (100 bytes)  bytes") {
		t.Fatalf("expected shortened hex:\n%s", out)
	}

	full := render(t, r, util.LayoutInfoFull, "")
	if strings.Count(full, "├─ 1") != 5 || !strings.Contains(full, "└─ 1") {
		t.Fatalf("expected expanded array in full layout:\n%s", full)
	}
	if !strings.Contains(full, "0x"+strings.Repeat("ab", 100)) {
		t.Fatalf("full layout must keep the whole hex blob:\n%s", full)
	}
}

func TestRevertedHeadlineAndPostSignLayout(t *testing.T) {
	r := swapTxResult()
	r.Status = "reverted"
	r.Logs = nil

	post := render(t, r, util.LayoutPostSign, hashHex)
	if !strings.HasPrefix(post, "✗ reverted   swapExactTokensForTokens  →  Uniswap V2 Router") {
		t.Fatalf("expected reverted headline first:\n%s", post)
	}
	if !strings.Contains(post, "gas 0.00213000 ETH (171203 / 185123)") {
		t.Fatalf("post-sign layout should show gas used vs limit:\n%s", post)
	}
	if !strings.Contains(post, "Call  swapExactTokensForTokens") {
		t.Fatalf("reverted tx must show the call in post-sign layout:\n%s", post)
	}

	r.Status = "done"
	r.Logs = swapTxResult().Logs
	post = render(t, r, util.LayoutPostSign, hashHex)
	if strings.Contains(post, "Call  ") {
		t.Fatalf("successful post-sign layout must not repeat the call:\n%s", post)
	}
	if !strings.Contains(post, "Transfers") || !strings.Contains(post, "Events (3)") {
		t.Fatalf("post-sign layout should keep transfers and events:\n%s", post)
	}
	if strings.Count(post, "✓ done") != 1 {
		t.Fatalf("post-sign layout should not repeat the headline as footer:\n%s", post)
	}
}

func TestPlainTransferIsHeadlineOnly(t *testing.T) {
	r := jarviscommon.NewTxResult()
	r.Status = "done"
	r.TxType = "normal"
	r.From = addr(meHex, "me")
	r.To = addr(routerHex, "")
	r.Value = "1.5"
	r.GasCost = "0.00031500"
	r.Nonce = "7"

	out := render(t, r, util.LayoutInfo, hashHex)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines for a plain transfer, got %d:\n%s", len(lines), out)
	}
	if lines[0] != "✓ done   transfer 1.5 ETH  →  0x7a25…488D (unknown)" {
		t.Fatalf("unexpected headline %q", lines[0])
	}
	if !strings.Contains(lines[1], "from me (0x9642…5D4E)   gas 0.00031500 ETH   nonce 7") {
		t.Fatalf("unexpected details %q", lines[1])
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
	if !rec.HasMessage("approves UNLIMITED") {
		t.Fatalf("expected UNLIMITED marker, got %v", rec.Entries())
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

func TestUnknownCallTargetIsWarnButParamsAreNot(t *testing.T) {
	r := swapTxResult()
	r.To = addr(routerHex, "unknown")
	r.FunctionCall.Destination = r.To
	rec := ui.NewRecordingUI()
	d := util.DisplayTxResult(rec, r, networks.EthereumMainnet, util.LayoutInfo, "")
	if d.To.Severity != ui.SeverityWarn {
		t.Fatalf("unknown call target should be Warn, got %v", d.To.Severity)
	}
	if d.From.Severity != ui.SeveritySuccess {
		t.Fatalf("known sender should be green, got %v", d.From.Severity)
	}
	r.From = addr(meHex, "")
	d = util.DisplayTxResult(rec, r, networks.EthereumMainnet, util.LayoutInfo, "")
	if d.From.Severity != ui.SeverityInfo {
		t.Fatalf("unknown sender should be plain, not yellow, got %v", d.From.Severity)
	}
	path := d.FunctionCall.Params[2]
	if path.Values[0].Severity != ui.SeveritySuccess {
		t.Fatalf("known param address should be green, got %v", path.Values[0].Severity)
	}
}
