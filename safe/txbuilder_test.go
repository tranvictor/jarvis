package safe

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tranvictor/jarvis/networks"
)

// exampleBuilderJSON is a verbatim Safe{Wallet} Transaction Builder export
// (the one at the repo root as safe_builder.example.json), inlined so the test
// stays pure and doesn't depend on a file outside the package.
const exampleBuilderJSON = `{"version": "1.0", "chainId": "56", "createdAt": 1785510919685, "meta": {"name": "Whitelist 0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001", "description": "Whitelist signer on chain 56", "txBuilderVersion": "2.0.1", "createdFromSafeAddress": "0xBEe1aA51DCe11FAa5BFC6C56dBFa5b95D4DFC000", "createdFromOwnerAddress": ""}, "transactions": [{"to": "0x1d9937e170Fc2174408581265bA0B87afDA4947F", "value": "0", "data": null, "contractMethod": {"inputs": [{"name": "settlementContract", "type": "address", "internalType": "address"}, {"name": "signingEoa", "type": "address", "internalType": "address"}], "name": "whitelist", "payable": false}, "contractInputsValues": {"settlementContract": "0x1af18F06F97679B16A8F553326ab2857e6cFd920", "signingEoa": "0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001"}}, {"to": "0x8A19578B3C19fBFa9FcF4cdDA6d94A9bfC2E587c", "value": "0", "data": null, "contractMethod": {"inputs": [{"name": "q", "type": "address", "internalType": "contract IPropAMMQuoteImpl"}, {"name": "dest", "type": "address", "internalType": "address"}], "name": "setQuoteImpl", "payable": false}, "contractInputsValues": {"q": "0x03c9d0247a239A2FDe83bA64a85dB5C2E26901B8", "dest": "0x1af18F06F97679B16A8F553326ab2857e6cFd920"}}]}`

// TestParseAndEncodeExampleBatch pins the exact calldata jarvis produces for a
// real export. These goldens are the whole point of the feature: if encoding
// ever drifts, owners would be signing something other than what the Safe UI
// showed them.
func TestParseAndEncodeExampleBatch(t *testing.T) {
	f, err := ParseTxBuilderJSON(exampleBuilderJSON)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}

	if got, err := f.ChainIDUint(); err != nil || got != 56 {
		t.Fatalf("chain id = %d, %v; want 56", got, err)
	}
	if got := f.SafeAddress(); got != "0xBEe1aA51DCe11FAa5BFC6C56dBFa5b95D4DFC000" {
		t.Errorf("safe address = %q", got)
	}
	if len(f.Transactions) != 2 {
		t.Fatalf("got %d transactions, want 2", len(f.Transactions))
	}

	calls, err := f.EncodeCalls(networks.BSCMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}

	want := []struct {
		to   string
		data string
	}{
		{
			to: "0x1d9937e170Fc2174408581265bA0B87afDA4947F",
			// whitelist(address,address)
			data: "b092145e" +
				"0000000000000000000000001af18f06f97679b16a8f553326ab2857e6cfd920" +
				"000000000000000000000000beeee605dc6a531aeb4bc3c809cf6dd86674f001",
		},
		{
			to: "0x8A19578B3C19fBFa9FcF4cdDA6d94A9bfC2E587c",
			// setQuoteImpl(address,address)
			data: "526b94e9" +
				"00000000000000000000000003c9d0247a239a2fde83ba64a85db5c2e26901b8" +
				"0000000000000000000000001af18f06f97679b16a8f553326ab2857e6cfd920",
		},
	}

	for i, w := range want {
		got := calls[i]
		if !strings.EqualFold(got.To.Hex(), w.to) {
			t.Errorf("call %d: to = %s, want %s", i, got.To.Hex(), w.to)
		}
		if got.Operation != 0 {
			t.Errorf("call %d: operation = %d, want 0 (CALL)", i, got.Operation)
		}
		if got.Value == nil || got.Value.Sign() != 0 {
			t.Errorf("call %d: value = %v, want 0", i, got.Value)
		}
		if gotHex := hex.EncodeToString(got.Data); gotHex != w.data {
			t.Errorf("call %d:\n got  %s\n want %s", i, gotHex, w.data)
		}
		if got.ABI == nil {
			t.Errorf("call %d: no synthesized ABI carried through", i)
		}
	}
}

func TestReadTxBuilderFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(exampleBuilderJSON), 0o644); err != nil {
		t.Fatalf("write: %s", err)
	}
	f, err := ReadTxBuilderFile(path)
	if err != nil {
		t.Fatalf("read from path: %s", err)
	}
	if len(f.Transactions) != 2 {
		t.Errorf("got %d transactions, want 2", len(f.Transactions))
	}

	if _, err := ReadTxBuilderFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("expected an error for a missing file")
	}
	if _, err := ReadTxBuilderFile("   "); err == nil {
		t.Error("expected an error for an empty path")
	}
}

// TestParseTxBuilderJSONRejectsNonJSON keeps the two flags from being confused:
// handing a path to --tx-builder-json must say so plainly instead of leaking a
// raw JSON syntax error.
func TestParseTxBuilderJSONRejectsNonJSON(t *testing.T) {
	for _, in := range []string{"", "   ", "./batch.json", "[1,2]", "not json"} {
		_, err := ParseTxBuilderJSON(in)
		if err == nil {
			t.Errorf("input %q: expected an error", in)
			continue
		}
		if in == "./batch.json" && !strings.Contains(err.Error(), "--tx-builder-file") {
			t.Errorf("a path passed as json should point at --tx-builder-file, got: %s", err)
		}
	}
}

// TestRawDataTransactionWins covers the Safe UI's "custom data" entries, where
// contractMethod is absent and the hex in `data` is authoritative.
func TestRawDataTransactionWins(t *testing.T) {
	f, err := ParseTxBuilderJSON(`{"version":"1.0","chainId":"1","meta":{},"transactions":[
	  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"1000","data":"0xdeadbeef"}
	]}`)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}
	calls, err := f.EncodeCalls(networks.EthereumMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	if got := hex.EncodeToString(calls[0].Data); got != "deadbeef" {
		t.Errorf("data = %s, want deadbeef", got)
	}
	if calls[0].Value.String() != "1000" {
		t.Errorf("value = %s, want 1000", calls[0].Value)
	}
}

// TestNativeTransferTransaction covers an entry with neither data nor
// contractMethod, which is only valid when it moves value.
func TestNativeTransferTransaction(t *testing.T) {
	f, err := ParseTxBuilderJSON(`{"version":"1.0","chainId":"1","meta":{},"transactions":[
	  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"5","data":null}
	]}`)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}
	calls, err := f.EncodeCalls(networks.EthereumMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	if len(calls[0].Data) != 0 {
		t.Errorf("expected empty calldata, got %x", calls[0].Data)
	}
}

func TestTxBuilderValidationRejects(t *testing.T) {
	cases := map[string]string{
		"empty batch": `{"version":"1.0","chainId":"1","meta":{},"transactions":[]}`,
		"missing to": `{"version":"1.0","chainId":"1","meta":{},"transactions":[
		  {"to":"","value":"0","data":"0xdead"}]}`,
		"bad to": `{"version":"1.0","chainId":"1","meta":{},"transactions":[
		  {"to":"not-an-address","value":"0","data":"0xdead"}]}`,
		"bad value": `{"version":"1.0","chainId":"1","meta":{},"transactions":[
		  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"1.5","data":"0xdead"}]}`,
		"negative value": `{"version":"1.0","chainId":"1","meta":{},"transactions":[
		  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"-1","data":"0xdead"}]}`,
		"nothing to do": `{"version":"1.0","chainId":"1","meta":{},"transactions":[
		  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"0","data":null}]}`,
		"method without name": `{"version":"1.0","chainId":"1","meta":{},"transactions":[
		  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"0","data":null,
		   "contractMethod":{"inputs":[],"name":"","payable":false},"contractInputsValues":{}}]}`,
	}
	for name, doc := range cases {
		if _, err := ParseTxBuilderJSON(doc); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// TestEncodeRejectsMissingInputValue is the case that matters most for safety:
// a declared input with no value must stop the batch, never encode as zero.
func TestEncodeRejectsMissingInputValue(t *testing.T) {
	f, err := ParseTxBuilderJSON(`{"version":"1.0","chainId":"1","meta":{},"transactions":[
	  {"to":"0x1d9937e170Fc2174408581265bA0B87afDA4947F","value":"0","data":null,
	   "contractMethod":{"inputs":[{"name":"who","type":"address","internalType":"address"}],
	                     "name":"whitelist","payable":false},
	   "contractInputsValues":{}}]}`)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}
	_, err = f.EncodeCalls(networks.EthereumMainnet)
	if err == nil {
		t.Fatal("expected an error for a missing input value")
	}
	if !strings.Contains(err.Error(), "who") {
		t.Errorf("error should name the missing input, got: %s", err)
	}
}

func TestChainIDUintRejectsGarbage(t *testing.T) {
	for _, id := range []string{"", "  ", "0x38", "fifty-six"} {
		f := &TxBuilderFile{ChainID: id}
		if _, err := f.ChainIDUint(); err == nil {
			t.Errorf("chainId %q: expected an error", id)
		}
	}
}

// TestSafeAddressAbsentOrInvalid documents that an unusable
// meta.createdFromSafeAddress reads as "the file doesn't say" — the caller then
// requires an explicit Safe address rather than silently targeting 0x0.
func TestSafeAddressAbsentOrInvalid(t *testing.T) {
	for _, addr := range []string{"", "   ", "0xnope", "not-an-address"} {
		f := &TxBuilderFile{Meta: TxBuilderMeta{CreatedFromSafeAddress: addr}}
		if got := f.SafeAddress(); got != "" {
			t.Errorf("createdFromSafeAddress %q -> %q, want empty", addr, got)
		}
	}
}

func TestTotalValueWei(t *testing.T) {
	f := &TxBuilderFile{Transactions: []TxBuilderTx{
		{Value: "10"}, {Value: "32"}, {Value: ""},
	}}
	total, err := f.TotalValueWei()
	if err != nil {
		t.Fatalf("total: %s", err)
	}
	if total.String() != "42" {
		t.Errorf("total = %s, want 42", total)
	}
}

// TestMergeCallABIsKeepsEveryMethodPerAddress covers the case that made a
// batch render as "<undecoded call>": four setters on one contract, where a
// single-ABI-per-address map kept only the last entry's method and every
// earlier selector became undecodable.
func TestMergeCallABIsKeepsEveryMethodPerAddress(t *testing.T) {
	const raw = `{
	  "version": "1.0",
	  "chainId": "1",
	  "meta": {"name": "Transactions Batch"},
	  "transactions": [
	    {"to": "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155", "value": "0", "data": null,
	     "contractMethod": {"inputs": [
	        {"name": "operator", "type": "address", "internalType": "address"},
	        {"name": "allowed", "type": "bool", "internalType": "bool"}],
	      "name": "setOperator", "payable": false},
	     "contractInputsValues": {"operator": "0x990bC4fE8B3f75Fe02F58b939a5fc4F76C725fc7", "allowed": true}},
	    {"to": "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155", "value": "0", "data": null,
	     "contractMethod": {"inputs": [
	        {"name": "newVT", "type": "address", "internalType": "address"}],
	      "name": "setVT", "payable": false},
	     "contractInputsValues": {"newVT": "0xbeeB9eeE061925cC6d2122F05a4e6536F0FEB000"}},
	    {"to": "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155", "value": "0", "data": null,
	     "contractMethod": {"inputs": [
	        {"name": "dest", "type": "address", "internalType": "address"},
	        {"name": "allowed", "type": "bool", "internalType": "bool"}],
	      "name": "setEthDestination", "payable": false},
	     "contractInputsValues": {"dest": "0x0FB7f20a75DFeD378D73a742c00197Ff7aB5172e", "allowed": true}},
	    {"to": "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155", "value": "0", "data": null,
	     "contractMethod": {"inputs": [
	        {"name": "dest", "type": "address", "internalType": "address"},
	        {"name": "allowed", "type": "bool", "internalType": "bool"}],
	      "name": "setTokenDestination", "payable": false},
	     "contractInputsValues": {"dest": "0x0FB7f20a75DFeD378D73a742c00197Ff7aB5172e", "allowed": true}}
	  ]
	}`

	f, err := ParseTxBuilderJSON(raw)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}
	calls, err := f.EncodeCalls(networks.EthereumMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}

	abis := MergeCallABIs(calls)
	if len(abis) != 1 {
		t.Fatalf("expected 1 address entry, got %d", len(abis))
	}
	merged := abis[strings.ToLower(calls[0].To.Hex())]
	if merged == nil {
		t.Fatal("no merged ABI for the batch target")
	}

	want := []string{"setOperator", "setVT", "setEthDestination", "setTokenDestination"}
	for i, c := range calls {
		m, err := merged.MethodById(c.Data[:4])
		if err != nil {
			t.Fatalf("call %d (%s): %s", i+1, want[i], err)
		}
		if m.Name != want[i] {
			t.Errorf("call %d decoded as %s, want %s", i+1, m.Name, want[i])
		}
	}
}

// TestMergeCallABIsDedupesRepeatedMethod keeps a batch that calls the same
// method twice from growing duplicate entries in the merged ABI.
func TestMergeCallABIsDedupesRepeatedMethod(t *testing.T) {
	const raw = `{
	  "version": "1.0",
	  "chainId": "1",
	  "meta": {"name": "b"},
	  "transactions": [
	    {"to": "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155", "value": "0", "data": null,
	     "contractMethod": {"inputs": [{"name": "newVT", "type": "address", "internalType": "address"}],
	      "name": "setVT", "payable": false},
	     "contractInputsValues": {"newVT": "0xbeeB9eeE061925cC6d2122F05a4e6536F0FEB000"}},
	    {"to": "0x1bD5af8e731D0969E8eBf7ea87f06d9Dc096d155", "value": "0", "data": null,
	     "contractMethod": {"inputs": [{"name": "newVT", "type": "address", "internalType": "address"}],
	      "name": "setVT", "payable": false},
	     "contractInputsValues": {"newVT": "0x990bC4fE8B3f75Fe02F58b939a5fc4F76C725fc7"}}
	  ]
	}`

	f, err := ParseTxBuilderJSON(raw)
	if err != nil {
		t.Fatalf("parse: %s", err)
	}
	calls, err := f.EncodeCalls(networks.EthereumMainnet)
	if err != nil {
		t.Fatalf("encode: %s", err)
	}
	merged := MergeCallABIs(calls)[strings.ToLower(calls[0].To.Hex())]
	if merged == nil {
		t.Fatal("no merged ABI")
	}
	if len(merged.Methods) != 1 {
		t.Errorf("merged ABI has %d methods, want 1", len(merged.Methods))
	}
	// The per-call ABIs must not have been mutated by the merge.
	for i, c := range calls {
		if len(c.ABI.Methods) != 1 {
			t.Errorf("call %d ABI has %d methods, want 1", i+1, len(c.ABI.Methods))
		}
	}
}
