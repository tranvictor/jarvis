package walletconnect

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestParseSendTxParams(t *testing.T) {
	raw := json.RawMessage(`[{
		"from": "0xAbC0000000000000000000000000000000000001",
		"to":   "0xDeF0000000000000000000000000000000000002",
		"value": "0xde0b6b3a7640000",
		"data":  "0xa9059cbb",
		"gas":   "0x30d40",
		"nonce": "0x5"
	}]`)
	tx, err := parseSendTxParams(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tx.From != "0xabc0000000000000000000000000000000000001" {
		t.Errorf("from not lowercased: %s", tx.From)
	}
	if tx.Value == nil || tx.Value.String() != "1000000000000000000" {
		t.Errorf("value = %v want 1e18", tx.Value)
	}
	if !bytes.Equal(tx.Data, []byte{0xa9, 0x05, 0x9c, 0xbb}) {
		t.Errorf("data = %x", tx.Data)
	}
	if tx.Gas != 200000 {
		t.Errorf("gas = %d want 200000", tx.Gas)
	}
	if !tx.NonceProvided || tx.Nonce != 5 {
		t.Errorf("nonce %d provided=%v", tx.Nonce, tx.NonceProvided)
	}
}

func TestParsePersonalSignParams_MessageFirst(t *testing.T) {
	raw := json.RawMessage(`["0x48656c6c6f", "0x00000000219ab540356cbb839cbe05303d7705fa"]`)
	got, err := parsePersonalSignParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Hello" {
		t.Errorf("got %q want %q", string(got), "Hello")
	}
}

func TestParsePersonalSignParams_AddressFirst(t *testing.T) {
	raw := json.RawMessage(`["0x00000000219ab540356cbb839cbe05303d7705fa", "0x48656c6c6f"]`)
	got, err := parsePersonalSignParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Hello" {
		t.Errorf("got %q want %q", string(got), "Hello")
	}
}

func TestParsePersonalSignParams_PlainString(t *testing.T) {
	raw := json.RawMessage(`["Please sign in", "0x00000000219ab540356cbb839cbe05303d7705fa"]`)
	got, err := parsePersonalSignParams(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "Please sign in" {
		t.Errorf("got %q", string(got))
	}
}

func TestParseSwitchChainParams(t *testing.T) {
	id, err := parseSwitchChainParams(json.RawMessage(`[{"chainId":"0x89"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if id != 137 {
		t.Errorf("id = %d want 137", id)
	}
}

func TestParseSignTypedDataV4_InlineObject(t *testing.T) {
	raw := json.RawMessage(`["0x00", {"primaryType":"Mail","domain":{"name":"x"}}]`)
	out, err := parseSignTypedDataV4Params(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte(`"primaryType":"Mail"`)) {
		t.Errorf("lost payload: %s", out)
	}
}

func TestParseSignTypedDataV4_WrappedString(t *testing.T) {
	raw := json.RawMessage(`["0x00", "{\"primaryType\":\"Mail\"}"]`)
	out, err := parseSignTypedDataV4Params(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"primaryType":"Mail"}` {
		t.Errorf("unwrap failed: %s", out)
	}
}

func TestParseSendTxParamsAuthorizationList(t *testing.T) {
	raw := json.RawMessage(`[{
		"from": "0xAbC0000000000000000000000000000000000001",
		"to":   "0xDeF0000000000000000000000000000000000002",
		"value": "0x0",
		"data": "0x",
		"authorizationList": [{
			"chainId": "0x1",
			"address": "0x1111111111111111111111111111111111111111",
			"nonce": "0x4",
			"yParity": "0x0",
			"r": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"s": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
		}]
	}]`)
	tx, err := parseSendTxParams(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(tx.Authorizations) != 1 {
		t.Fatalf("auths %d", len(tx.Authorizations))
	}
	a := tx.Authorizations[0]
	if a.Address.Hex() != "0x1111111111111111111111111111111111111111" {
		t.Errorf("address %s", a.Address.Hex())
	}
	if a.Nonce != 4 || a.ChainID.Uint64() != 1 || a.V != 0 {
		t.Errorf("nonce=%d chain=%s v=%d", a.Nonce, a.ChainID.String(), a.V)
	}

	raw = json.RawMessage(`[{
		"from": "0xAbC0000000000000000000000000000000000001",
		"to":   "0xDeF0000000000000000000000000000000000002",
		"authorizationList": [{
			"chainId": "1",
			"address": "0x0000000000000000000000000000000000000000",
			"nonce": "5",
			"v": "28",
			"r": "0x1",
			"s": "0x2"
		}]
	}]`)
	tx, err = parseSendTxParams(raw)
	if err != nil {
		t.Fatalf("tolerant parse: %v", err)
	}
	if len(tx.Authorizations) != 1 {
		t.Fatalf("auths %d", len(tx.Authorizations))
	}
	a = tx.Authorizations[0]
	if a.Address.Hex() != "0x0000000000000000000000000000000000000000" {
		t.Errorf("revoke address %s", a.Address.Hex())
	}
	if a.V != 1 {
		t.Errorf("v=28 should become yParity 1, got %d", a.V)
	}
}
