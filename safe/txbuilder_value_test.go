package safe

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"

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
