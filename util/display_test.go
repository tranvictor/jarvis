package util_test

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/ui"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/addrbook"
)

const nestedLayerABI = `[{
	"type": "function",
	"name": "nestedLayer",
	"inputs": [],
	"outputs": [
		{"name": "num", "type": "uint256"},
		{"name": "account", "type": "address"},
		{
			"name": "secondLayer", "type": "tuple",
			"internalType": "struct TestNestedReturnSecondLayer",
			"components": [
				{"name": "id", "type": "uint256"},
				{
					"name": "layers", "type": "tuple[]",
					"internalType": "struct TestNestedReturnLayer[]",
					"components": [
						{"name": "index", "type": "uint256"},
						{"name": "owner", "type": "address"},
						{"name": "text", "type": "string"}
					]
				}
			]
		},
		{
			"name": "value", "type": "tuple",
			"internalType": "struct TestNestedReturnSomeValues",
			"components": [
				{"name": "firstVal", "type": "uint256"},
				{"name": "secondVal", "type": "uint256"},
				{"name": "addrVal", "type": "address"}
			]
		}
	],
	"stateMutability": "view"
}]`

type testLayer struct {
	Index *big.Int
	Owner ethcommon.Address
	Text  string
}

type testSecondLayer struct {
	Id     *big.Int
	Layers []testLayer
}

type testSomeValues struct {
	FirstVal  *big.Int
	SecondVal *big.Int
	AddrVal   ethcommon.Address
}

// nestedLayerFixture packs, unpacks, and converts the test data into
// []jarviscommon.ParamResult — the same pipeline that the real command uses
// between ReadContractToBytes and DisplayParams.
func nestedLayerFixture(t *testing.T) []jarviscommon.ParamResult {
	t.Helper()

	parsed, err := abi.JSON(strings.NewReader(nestedLayerABI))
	if err != nil {
		t.Fatalf("parse ABI: %s", err)
	}
	method := parsed.Methods["nestedLayer"]

	packed, err := method.Outputs.Pack(
		big.NewInt(1000),
		ethcommon.HexToAddress("0x9642b23Ed1E01Df1092B92641051881a322F5D4E"),
		testSecondLayer{
			Id: big.NewInt(10),
			Layers: []testLayer{
				{Index: big.NewInt(1), Owner: ethcommon.HexToAddress("0x4838B106FCe9647Bdf1E7877BF73cE8B0BAD5f97"), Text: "Hello 1"},
				{Index: big.NewInt(2), Owner: ethcommon.HexToAddress("0x9642b23Ed1E01Df1092B92641051881a322F5D4E"), Text: "Hello 2"},
			},
		},
		testSomeValues{
			FirstVal:  big.NewInt(16),
			SecondVal: big.NewInt(30),
			AddrVal:   ethcommon.HexToAddress("0x559432E18b281731c054cD703D4B49872BE4ed53"),
		},
	)
	if err != nil {
		t.Fatalf("pack outputs: %s", err)
	}

	unpacked, err := method.Outputs.UnpackValues(packed)
	if err != nil {
		t.Fatalf("unpack outputs: %s", err)
	}

	resolver := addrbook.Map{}
	ctx := txanalyzer.NewAnalysisContextWithResolver(nil, networks.BSCMainnet, resolver)
	analyzer := txanalyzer.NewGenericAnalyzerWithContext(ctx)

	var params []jarviscommon.ParamResult
	for i, output := range method.Outputs {
		params = append(params, analyzer.ParamAsJarvisParamResult(output.Name, output.Type, unpacked[i]))
	}
	return params
}

// ---------------------------------------------------------------------------
// Test 1: output values (data correctness)
// ---------------------------------------------------------------------------

func TestNestedLayerValues(t *testing.T) {
	rec := ui.NewRecordingUI()
	params := nestedLayerFixture(t)
	displays := util.DisplayParams(rec, params)

	if len(displays) != 4 {
		t.Fatalf("expected 4 top-level params, got %d", len(displays))
	}

	// --- num ---
	assertParam(t, displays[0], "num", "uint256")
	assertScalarValue(t, displays[0], "1000")

	// --- account ---
	assertParam(t, displays[1], "account", "address")
	assertScalarContains(t, displays[1], "0x9642b23Ed1E01Df1092B92641051881a322F5D4E")

	// --- secondLayer ---
	assertParam(t, displays[2], "secondLayer", "TestNestedReturnSecondLayer")
	if len(displays[2].Tuples) != 1 {
		t.Fatalf("secondLayer: expected 1 tuple, got %d", len(displays[2].Tuples))
	}
	slFields := displays[2].Tuples[0].Fields
	if len(slFields) != 2 {
		t.Fatalf("secondLayer: expected 2 fields, got %d", len(slFields))
	}

	// secondLayer.id
	assertParam(t, slFields[0], "id", "uint256")
	assertScalarValue(t, slFields[0], "10")

	// secondLayer.layers
	assertParam(t, slFields[1], "layers", "(uint256,address,string)[]")
	if len(slFields[1].Tuples) != 2 {
		t.Fatalf("layers: expected 2 tuples, got %d", len(slFields[1].Tuples))
	}

	// layers[0]
	l0 := slFields[1].Tuples[0].Fields
	assertScalarValue(t, l0[0], "1")
	assertScalarContains(t, l0[1], "0x4838B106FCe9647Bdf1E7877BF73cE8B0BAD5f97")
	assertScalarValue(t, l0[2], "Hello 1")

	// layers[1]
	l1 := slFields[1].Tuples[1].Fields
	assertScalarValue(t, l1[0], "2")
	assertScalarContains(t, l1[1], "0x9642b23Ed1E01Df1092B92641051881a322F5D4E")
	assertScalarValue(t, l1[2], "Hello 2")

	// --- value ---
	assertParam(t, displays[3], "value", "TestNestedReturnSomeValues")
	if len(displays[3].Tuples) != 1 {
		t.Fatalf("value: expected 1 tuple, got %d", len(displays[3].Tuples))
	}
	vFields := displays[3].Tuples[0].Fields
	assertScalarValue(t, vFields[0], "16")
	assertScalarValue(t, vFields[1], "30")
	assertScalarContains(t, vFields[2], "0x559432E18b281731c054cD703D4B49872BE4ed53")
}

// ---------------------------------------------------------------------------
// Test 2: UI representation (RecordingUI entries)
// ---------------------------------------------------------------------------

func TestNestedLayerUIRepresentation(t *testing.T) {
	rec := ui.NewRecordingUI()
	params := nestedLayerFixture(t)
	util.DisplayParams(rec, params)

	entries := rec.Entries()
	tableRows := filterTableEntries(entries)

	// The table should start with a header and contain the expected groups
	// separated by "---" dividers.
	expected := []string{
		"Parameter | Value",
		// Group 1: scalar params
		"num (uint256) | 1000",
		"account (address) | 0x9642b23Ed1E01Df1092B92641051881a322F5D4E (unknown)",
		"---",
		// Group 2: secondLayer
		"secondLayer (TestNestedReturnSecondLayer) | ",
		"  id (uint256) | 10",
		"  layers ((uint256,address,string)[]) | ",
		"    [0] index (uint256) | 1",
		"        owner (address) | 0x4838B106FCe9647Bdf1E7877BF73cE8B0BAD5f97 (unknown)",
		"        text (string) | Hello 1",
		"    [1] index (uint256) | 2",
		"        owner (address) | 0x9642b23Ed1E01Df1092B92641051881a322F5D4E (unknown)",
		"        text (string) | Hello 2",
		"---",
		// Group 3: value
		"value (TestNestedReturnSomeValues) | ",
		"  firstVal (uint256) | 16",
		"  secondVal (uint256) | 30",
		"  addrVal (address) | 0x559432E18b281731c054cD703D4B49872BE4ed53 (unknown)",
	}

	if len(tableRows) != len(expected) {
		t.Errorf("expected %d table entries, got %d", len(expected), len(tableRows))
		for i, row := range tableRows {
			t.Logf("  [%d] %q", i, row)
		}
		t.FailNow()
	}

	for i, want := range expected {
		if tableRows[i] != want {
			t.Errorf("row %d:\n  want: %q\n   got: %q", i, want, tableRows[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func filterTableEntries(entries []ui.Entry) []string {
	var rows []string
	for _, e := range entries {
		if e.Method == "Table" {
			rows = append(rows, e.Value)
		}
	}
	return rows
}

func assertParam(t *testing.T, d util.ParamDisplay, name, typ string) {
	t.Helper()
	if d.Name != name {
		t.Errorf("Name: want %q, got %q", name, d.Name)
	}
	if d.Type != typ {
		t.Errorf("Type: want %q, got %q", typ, d.Type)
	}
}

func assertScalarValue(t *testing.T, d util.ParamDisplay, want string) {
	t.Helper()
	if len(d.Values) == 0 {
		t.Fatalf("param %q: expected scalar values, got none", d.Name)
	}
	got := d.Values[0].Text
	if got != want {
		t.Errorf("param %q: want value %q, got %q", d.Name, want, got)
	}
}

func assertScalarContains(t *testing.T, d util.ParamDisplay, substr string) {
	t.Helper()
	if len(d.Values) == 0 {
		t.Fatalf("param %q: expected scalar values, got none", d.Name)
	}
	got := d.Values[0].Text
	if !strings.Contains(got, substr) {
		t.Errorf("param %q: want value containing %q, got %q", d.Name, substr, got)
	}
}
