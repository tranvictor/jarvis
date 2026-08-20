package safe

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/networks"
)

func mustType(t *testing.T, solidityType string, components ...abi.ArgumentMarshaling) abi.Type {
	t.Helper()
	ty, err := abi.NewType(solidityType, "", components)
	if err != nil {
		t.Fatalf("abi.NewType(%q): %s", solidityType, err)
	}
	return ty
}

// TestBuilderValueToJarvisInput pins the translation from Safe-UI JSON values
// into the input syntax jarvis's own converters accept. Getting this wrong
// silently changes calldata, so every supported shape is nailed down.
func TestBuilderValueToJarvisInput(t *testing.T) {
	tupleComponents := []abi.ArgumentMarshaling{
		{Name: "target", Type: "address"},
		{Name: "amount", Type: "uint256"},
	}

	cases := []struct {
		name       string
		typ        abi.Type
		raw        string
		want       string
		wantErrSub string
	}{
		{
			name: "address passes through",
			typ:  mustType(t, "address"),
			raw:  "0x1af18F06F97679B16A8F553326ab2857e6cFd920",
			want: "0x1af18F06F97679B16A8F553326ab2857e6cFd920",
		},
		{
			name: "uint256 passes through",
			typ:  mustType(t, "uint256"),
			raw:  "1000000000000000000",
			want: "1000000000000000000",
		},
		{
			name: "bool passes through",
			typ:  mustType(t, "bool"),
			raw:  "true",
			want: "true",
		},
		{
			name: "bytes passes through",
			typ:  mustType(t, "bytes"),
			raw:  "0xdeadbeef",
			want: "0xdeadbeef",
		},
		{
			// jarvis's util.ConvertToString requires the quotes; the Safe UI
			// never writes them.
			name: "string gets quoted",
			typ:  mustType(t, "string"),
			raw:  "hello world",
			want: `"hello world"`,
		},
		{
			// A value the operator deliberately quoted survives, because
			// jarvis strips exactly one layer.
			name: "already-quoted string keeps its quotes",
			typ:  mustType(t, "string"),
			raw:  `"quoted"`,
			want: `""quoted""`,
		},
		{
			name: "address array loses JSON quotes",
			typ:  mustType(t, "address[]"),
			raw:  `["0x1af18F06F97679B16A8F553326ab2857e6cFd920","0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001"]`,
			want: "[0x1af18F06F97679B16A8F553326ab2857e6cFd920,0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001]",
		},
		{
			name: "uint256 array from JSON numbers",
			typ:  mustType(t, "uint256[]"),
			raw:  `[1,2,3]`,
			want: "[1,2,3]",
		},
		{
			// float64 would round this; UseNumber keeps the exact digits.
			name: "large uint256 array keeps full precision",
			typ:  mustType(t, "uint256[]"),
			raw:  `[115792089237316195423570985008687907853269984665640564039457584007913129639935]`,
			want: "[115792089237316195423570985008687907853269984665640564039457584007913129639935]",
		},
		{
			name: "fixed-size array",
			typ:  mustType(t, "uint8[2]"),
			raw:  `[7,9]`,
			want: "[7,9]",
		},
		{
			name:       "fixed-size array with wrong arity",
			typ:        mustType(t, "uint8[2]"),
			raw:        `[7,9,11]`,
			wantErrSub: "expected 2 elements",
		},
		{
			name: "string array gets each element quoted",
			typ:  mustType(t, "string[]"),
			raw:  `["a","b"]`,
			want: `["a","b"]`,
		},
		{
			name: "nested array",
			typ:  mustType(t, "uint256[][]"),
			raw:  `[[1,2],[3]]`,
			want: "[[1,2],[3]]",
		},
		{
			name: "bool array",
			typ:  mustType(t, "bool[]"),
			raw:  `[true,false]`,
			want: "[true,false]",
		},
		{
			name: "tuple from JSON array becomes parens",
			typ:  mustType(t, "tuple", tupleComponents...),
			raw:  `["0x1af18F06F97679B16A8F553326ab2857e6cFd920",42]`,
			want: "(0x1af18F06F97679B16A8F553326ab2857e6cFd920,42)",
		},
		{
			name: "tuple from JSON object is ordered by component name",
			typ:  mustType(t, "tuple", tupleComponents...),
			raw:  `{"amount":42,"target":"0x1af18F06F97679B16A8F553326ab2857e6cFd920"}`,
			want: "(0x1af18F06F97679B16A8F553326ab2857e6cFd920,42)",
		},
		{
			name:       "tuple object missing a field",
			typ:        mustType(t, "tuple", tupleComponents...),
			raw:        `{"amount":42}`,
			wantErrSub: "missing field",
		},
		{
			name:       "tuple array with wrong arity",
			typ:        mustType(t, "tuple", tupleComponents...),
			raw:        `["0x1af18F06F97679B16A8F553326ab2857e6cFd920"]`,
			wantErrSub: "expected 2 fields",
		},
		{
			name: "tuple array type",
			typ:  mustType(t, "tuple[]", tupleComponents...),
			raw:  `[["0x1af18F06F97679B16A8F553326ab2857e6cFd920",1],["0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001",2]]`,
			want: "[(0x1af18F06F97679B16A8F553326ab2857e6cFd920,1),(0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001,2)]",
		},
		{
			// jarvis's array splitter isn't quote-aware, so a comma inside an
			// element would silently change the decoded value. Refuse instead.
			name:       "array element containing a comma is rejected",
			typ:        mustType(t, "string[]"),
			raw:        `["a,b","c"]`,
			wantErrSub: "cannot parse inside an array",
		},
		{
			name:       "empty array element is rejected",
			typ:        mustType(t, "string[]"),
			raw:        `["",""]`,
			wantErrSub: "empty values inside arrays",
		},
		{
			name:       "null array element is rejected",
			typ:        mustType(t, "address[]"),
			raw:        `[null]`,
			wantErrSub: "null",
		},
		{
			// A hand-edited file may already use jarvis syntax, which isn't
			// valid JSON. Pass it through rather than failing on a form that
			// works downstream.
			name: "non-JSON array falls through unchanged",
			typ:  mustType(t, "address[]"),
			raw:  "[0x1af18F06F97679B16A8F553326ab2857e6cFd920]",
			want: "[0x1af18F06F97679B16A8F553326ab2857e6cFd920]",
		},
		{
			name:       "array given a JSON scalar",
			typ:        mustType(t, "uint256[]"),
			raw:        `5`,
			wantErrSub: "expected a JSON array",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := builderValueToJarvisInput(c.typ, c.raw)
			if c.wantErrSub != "" {
				if err == nil {
					t.Fatalf("expected an error containing %q, got %q", c.wantErrSub, got)
				}
				if !strings.Contains(err.Error(), c.wantErrSub) {
					t.Fatalf("error %q doesn't contain %q", err, c.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestNormalizedValuesActuallyPack closes the loop: the normaliser's output
// must be something util.ConvertParamStrToType accepts and go-ethereum can
// pack. A normaliser that produced pretty-but-unusable strings would pass the
// table test above and still break every real batch.
func TestNormalizedValuesActuallyPack(t *testing.T) {
	f, err := ParseTxBuilderJSON(`{"version":"1.0","chainId":"1","meta":{},"transactions":[
	  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"0","data":null,
	   "contractMethod":{"name":"configure","payable":false,"inputs":[
	     {"name":"label","type":"string","internalType":"string"},
	     {"name":"targets","type":"address[]","internalType":"address[]"},
	     {"name":"limits","type":"uint256[]","internalType":"uint256[]"},
	     {"name":"enabled","type":"bool","internalType":"bool"},
	     {"name":"extra","type":"bytes","internalType":"bytes"},
	     {"name":"salt","type":"bytes32","internalType":"bytes32"},
	     {"name":"cap","type":"tuple","internalType":"struct Cap","components":[
	        {"name":"target","type":"address","internalType":"address"},
	        {"name":"amount","type":"uint256","internalType":"uint256"}]}]},
	   "contractInputsValues":{
	     "label":"prod rollout",
	     "targets":"[\"0x1af18F06F97679B16A8F553326ab2857e6cFd920\",\"0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001\"]",
	     "limits":"[1,115792089237316195423570985008687907853269984665640564039457584007913129639935]",
	     "enabled":"true",
	     "extra":"0xdeadbeef",
	     "salt":"0x00000000000000000000000000000000000000000000000000000000000000ff",
	     "cap":"[\"0x1af18F06F97679B16A8F553326ab2857e6cFd920\",7]"}}]}`)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}

	calls, err := f.EncodeCalls(networks.EthereumMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	if len(calls[0].Data) < 4 {
		t.Fatalf("calldata too short: %x", calls[0].Data)
	}

	// Round-trip through the synthesized ABI: whatever was packed must
	// unpack back to the same values.
	a := calls[0].ABI
	if a == nil {
		t.Fatal("no synthesized ABI")
	}
	m, err := a.MethodById(calls[0].Data[:4])
	if err != nil {
		t.Fatalf("method lookup: %s", err)
	}
	values, err := m.Inputs.UnpackValues(calls[0].Data[4:])
	if err != nil {
		t.Fatalf("unpack: %s", err)
	}
	if len(values) != 7 {
		t.Fatalf("got %d values, want 7", len(values))
	}
	if values[0] != "prod rollout" {
		t.Errorf("label = %#v", values[0])
	}
	if values[3] != true {
		t.Errorf("enabled = %#v", values[3])
	}
	if got, ok := values[4].([]byte); !ok || hex.EncodeToString(got) != "deadbeef" {
		t.Errorf("extra = %#v", values[4])
	}
}

// TestBareJSONLiteralValues covers exports where the Safe UI wrote a value as
// a bare JSON literal instead of a quoted string — `"allowed": true` rather
// than `"allowed": "true"`. Those files used to fail at unmarshal time with
// "cannot unmarshal bool into Go struct field ... of type string", which told
// the operator nothing about which entry was at fault and, worse, rejected a
// batch the Safe UI itself produced.
func TestBareJSONLiteralValues(t *testing.T) {
	f, err := ParseTxBuilderJSON(`{"version":"1.0","chainId":"1","createdAt":1787138928141,
	  "meta":{"name":"Transactions Batch","txBuilderVersion":"2.0.1",
	    "createdFromSafeAddress":"0x5B8c76E2a97746f375F629bDbf54B0e4FF19b803"},
	  "transactions":[
	  {"to":"0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155","value":"0","data":null,
	   "contractMethod":{"name":"setOperator","payable":false,"inputs":[
	     {"name":"operator","type":"address","internalType":"address"},
	     {"name":"allowed","type":"bool","internalType":"bool"}]},
	   "contractInputsValues":{
	     "operator":"0xFf017006107E0255aD786Ab4CF92855448F605fc",
	     "allowed":true}},
	  {"to":"0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155","value":"0","data":null,
	   "contractMethod":{"name":"setLimits","payable":false,"inputs":[
	     {"name":"cap","type":"uint256","internalType":"uint256"},
	     {"name":"enabled","type":"bool","internalType":"bool"},
	     {"name":"targets","type":"address[]","internalType":"address[]"}]},
	   "contractInputsValues":{
	     "cap":115792089237316195423570985008687907853269984665640564039457584007913129639935,
	     "enabled":false,
	     "targets":["0x1af18F06F97679B16A8F553326ab2857e6cFd920"]}}]}`)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}

	calls, err := f.EncodeCalls(networks.EthereumMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}

	first := unpackCall(t, calls[0])
	if got := first[0].(gethcommon.Address); got != gethcommon.HexToAddress("0xFf017006107E0255aD786Ab4CF92855448F605fc") {
		t.Errorf("operator = %s", got.Hex())
	}
	if first[1] != true {
		t.Errorf("allowed = %#v, want true", first[1])
	}

	second := unpackCall(t, calls[1])
	// A bare JSON number above 2^53: it must survive digit for digit, which
	// is why the raw text is kept instead of decoding into float64.
	wantCap, _ := new(big.Int).SetString(
		"115792089237316195423570985008687907853269984665640564039457584007913129639935", 10,
	)
	if got, ok := second[0].(*big.Int); !ok || got.Cmp(wantCap) != 0 {
		t.Errorf("cap = %#v, want %s", second[0], wantCap)
	}
	if second[1] != false {
		t.Errorf("enabled = %#v, want false", second[1])
	}
	if got, ok := second[2].([]gethcommon.Address); !ok || len(got) != 1 ||
		got[0] != gethcommon.HexToAddress("0x1af18F06F97679B16A8F553326ab2857e6cFd920") {
		t.Errorf("targets = %#v", second[2])
	}
}

// TestTxBuilderValueRoundTrip pins that re-serialising a parsed batch gives
// back the spelling the file used, so a bare literal never turns into a
// string (or the other way round) behind the operator's back.
func TestTxBuilderValueRoundTrip(t *testing.T) {
	var v TxBuilderValue
	for _, raw := range []string{`"true"`, `true`, `123`, `["0xaa"]`, `null`} {
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatalf("unmarshal %s: %s", raw, err)
		}
		out, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %s: %s", raw, err)
		}
		if string(out) != raw {
			t.Errorf("round trip of %s gave %s", raw, out)
		}
	}
}

// TestTxBuilderValueText documents the normalisation both spellings go
// through: quoted values are unquoted, bare literals are kept verbatim, and
// null degrades to the empty string the old map[string]string produced.
func TestTxBuilderValueText(t *testing.T) {
	cases := map[string]string{
		`"true"`:         "true",
		`true`:           "true",
		`"0xdeadbeef"`:   "0xdeadbeef",
		`123`:            "123",
		`  false `:       "false",
		`"[\"0xaa\",1]"`: `["0xaa",1]`,
		`["0xaa",1]`:     `["0xaa",1]`,
		`null`:           "",
		`"prod rollout"`: "prod rollout",
	}
	for raw, want := range cases {
		var v TxBuilderValue
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatalf("unmarshal %s: %s", raw, err)
		}
		if got := v.String(); got != want {
			t.Errorf("%s -> %q, want %q", raw, got, want)
		}
	}
}

func unpackCall(t *testing.T, call TxBuilderCall) []any {
	t.Helper()
	if call.ABI == nil {
		t.Fatal("no synthesized ABI")
	}
	if len(call.Data) < 4 {
		t.Fatalf("calldata too short: %x", call.Data)
	}
	m, err := call.ABI.MethodById(call.Data[:4])
	if err != nil {
		t.Fatalf("method lookup: %s", err)
	}
	values, err := m.Inputs.UnpackValues(call.Data[4:])
	if err != nil {
		t.Fatalf("unpack: %s", err)
	}
	return values
}
