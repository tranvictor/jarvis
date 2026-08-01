package common

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestMultiSendSelectorMatchesABI(t *testing.T) {
	m, ok := GetMultiSendABI().Methods[MultiSendMethodName]
	if !ok {
		t.Fatalf("built-in ABI has no %s method", MultiSendMethodName)
	}
	if !bytes.Equal(m.ID, MultiSendSelector) {
		t.Fatalf("hardcoded selector %x != ABI selector %x", MultiSendSelector, m.ID)
	}
}

func TestEncodeDecodeMultiSendPayloadRoundTrip(t *testing.T) {
	calls := []MultiSendCall{
		{
			Operation: 0,
			To:        common.HexToAddress("0x1d9937e170Fc2174408581265bA0B87afDA4947F"),
			Value:     big.NewInt(0),
			Data:      []byte{0xb0, 0x92, 0x14, 0x5e, 0x01, 0x02},
		},
		{
			// No data and a huge value: a plain native-token transfer.
			Operation: 0,
			To:        common.HexToAddress("0xBeEEE605DC6a531AeB4bc3C809Cf6Dd86674F001"),
			Value:     mustBig(t, "115792089237316195423570985008687907853269984665640564039457584007913129639935"),
			Data:      nil,
		},
		{
			// Nil value must round-trip as zero rather than panicking.
			Operation: 1,
			To:        common.HexToAddress("0x8A19578B3C19fBFa9FcF4cdDA6d94A9bfC2E587c"),
			Value:     nil,
			Data:      bytes.Repeat([]byte{0xab}, 200),
		},
	}

	decoded, err := DecodeMultiSendPayload(EncodeMultiSendPayload(calls))
	if err != nil {
		t.Fatalf("decode: %s", err)
	}
	if len(decoded) != len(calls) {
		t.Fatalf("got %d calls, want %d", len(decoded), len(calls))
	}
	for i, want := range calls {
		got := decoded[i]
		if got.Operation != want.Operation {
			t.Errorf("call %d: operation %d != %d", i, got.Operation, want.Operation)
		}
		if got.To != want.To {
			t.Errorf("call %d: to %s != %s", i, got.To.Hex(), want.To.Hex())
		}
		wantValue := want.Value
		if wantValue == nil {
			wantValue = big.NewInt(0)
		}
		if got.Value.Cmp(wantValue) != 0 {
			t.Errorf("call %d: value %s != %s", i, got.Value, wantValue)
		}
		if !bytes.Equal(got.Data, want.Data) && !(len(got.Data) == 0 && len(want.Data) == 0) {
			t.Errorf("call %d: data %x != %x", i, got.Data, want.Data)
		}
	}
}

func TestEncodeMultiSendPayloadLayout(t *testing.T) {
	payload := EncodeMultiSendPayload([]MultiSendCall{{
		Operation: 0,
		To:        common.HexToAddress("0x000000000000000000000000000000000000dEaD"),
		Value:     big.NewInt(1),
		Data:      []byte{0xaa, 0xbb},
	}})

	// operation(1) + to(20) + value(32) + length(32) + data(2)
	if len(payload) != 1+20+32+32+2 {
		t.Fatalf("unexpected payload length %d", len(payload))
	}
	if payload[0] != 0 {
		t.Errorf("operation byte = %d, want 0", payload[0])
	}
	if got := common.BytesToAddress(payload[1:21]); got != common.HexToAddress("0xdEaD") {
		t.Errorf("to = %s", got.Hex())
	}
	if got := new(big.Int).SetBytes(payload[21:53]); got.Cmp(big.NewInt(1)) != 0 {
		t.Errorf("value = %s, want 1", got)
	}
	if got := new(big.Int).SetBytes(payload[53:85]); got.Cmp(big.NewInt(2)) != 0 {
		t.Errorf("data length = %s, want 2", got)
	}
	if !bytes.Equal(payload[85:], []byte{0xaa, 0xbb}) {
		t.Errorf("data = %x", payload[85:])
	}
}

func TestDecodeMultiSendPayloadRejectsMalformed(t *testing.T) {
	good := EncodeMultiSendPayload([]MultiSendCall{{
		To:    common.HexToAddress("0x000000000000000000000000000000000000dEaD"),
		Value: big.NewInt(0),
		Data:  []byte{0xaa, 0xbb},
	}})

	cases := map[string][]byte{
		"empty":                    {},
		"truncated header":         good[:40],
		"truncated data":           good[:len(good)-1],
		"trailing partial entry":   append(append([]byte{}, good...), 0x00, 0x01),
		"data length overruns buf": overstateLength(good),
	}

	for name, payload := range cases {
		if _, err := DecodeMultiSendPayload(payload); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestPackMultiSendRejectsEmptyBatch(t *testing.T) {
	if _, err := PackMultiSend(nil); err == nil {
		t.Fatal("expected an error for an empty batch")
	}
}

func TestIsMultiSendCallData(t *testing.T) {
	packed, err := PackMultiSend([]MultiSendCall{{
		To:    common.HexToAddress("0x000000000000000000000000000000000000dEaD"),
		Value: big.NewInt(0),
	}})
	if err != nil {
		t.Fatalf("pack: %s", err)
	}
	if !IsMultiSendCallData(packed) {
		t.Error("packed multiSend calldata not recognised")
	}
	if IsMultiSendCallData([]byte{0xb0, 0x92, 0x14, 0x5e}) {
		t.Error("unrelated selector recognised as multiSend")
	}
	if IsMultiSendCallData([]byte{0x8d, 0x80}) {
		t.Error("truncated selector recognised as multiSend")
	}
}

// overstateLength rewrites the dataLength word of the first entry (bytes
// 53..85, after operation+to+value) so it claims more bytes than the buffer
// holds.
func overstateLength(payload []byte) []byte {
	out := append([]byte{}, payload...)
	for i := 53; i < 85; i++ {
		out[i] = 0
	}
	out[84] = 0xff
	return out
}

func mustBig(t *testing.T, s string) *big.Int {
	t.Helper()
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big int %q", s)
	}
	return v
}
