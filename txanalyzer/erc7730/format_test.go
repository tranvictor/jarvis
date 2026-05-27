package erc7730

import (
	"math/big"
	"testing"
)

type stubHelpers struct{}

func (stubHelpers) ResolveAddress(addr string, _ uint64) string {
	if addr == "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48" {
		return "0xa0b8…6eB48 (USDC)"
	}
	return addr
}
func (stubHelpers) TokenMetadata(addr string, _ uint64) (uint64, string, bool) {
	if addr == "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48" {
		return 6, "USDC", true
	}
	return 0, "", false
}
func (stubHelpers) NetworkName(id uint64) (string, bool) {
	if id == 1 {
		return "Ethereum Mainnet", true
	}
	return "", false
}
func (stubHelpers) NativeSymbol(id uint64) string {
	if id == 1 {
		return "ETH"
	}
	return ""
}

func TestTokenAmountResolvesViaTokenPath(t *testing.T) {
	data := ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{
			{Name: "token", Value: addrValue("0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")},
			{Name: "value", Value: intValue(new(big.Int).SetUint64(1_000_000))},
		},
	}
	r := &Resolver{Data: data, Container: Container{ChainID: 1}}
	f := &Formatter{Resolver: r, Helpers: stubHelpers{}}
	rows, err := f.FormatField(Field{
		Path: "value", Label: "Amount", Format: "tokenAmount",
		Params: map[string]any{"tokenPath": "token"},
	})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != "1 USDC" {
		t.Errorf("got rows=%v", rows)
	}
}

func TestTokenAmountThreshold(t *testing.T) {
	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	data := ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{
			{Name: "token", Value: addrValue("0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")},
			{Name: "value", Value: intValue(max)},
		},
	}
	r := &Resolver{Data: data, Container: Container{ChainID: 1}}
	f := &Formatter{Resolver: r, Helpers: stubHelpers{}}
	rows, err := f.FormatField(Field{
		Path: "value", Label: "Amount", Format: "tokenAmount",
		Params: map[string]any{
			"tokenPath": "token",
			"threshold": "0x8000000000000000000000000000000000000000000000000000000000000000",
		},
	})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != "Unlimited USDC" {
		t.Errorf("threshold render: got %q", rows[0].Value)
	}
}

func TestDateFormatTimestamp(t *testing.T) {
	r := &Resolver{Data: ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{{Name: "deadline", Value: intValue(big.NewInt(1709191632))}},
	}}
	f := &Formatter{Resolver: r}
	rows, _ := f.FormatField(Field{Path: "deadline", Format: "date",
		Params: map[string]any{"encoding": "timestamp"}})
	if len(rows) != 1 || rows[0].Value != "2024-02-29T07:27:12Z" {
		t.Errorf("date render: got %q", rows[0].Value)
	}
}

func TestEnumFormat(t *testing.T) {
	desc := &Descriptor{Metadata: Metadata{Enums: map[string]Enum{
		"mode": {"1": "stable", "2": "variable"},
	}}}
	r := &Resolver{Descriptor: desc, Data: ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{{Name: "mode", Value: intValue(big.NewInt(2))}},
	}}
	f := &Formatter{Resolver: r}
	rows, _ := f.FormatField(Field{Path: "mode", Format: "enum",
		Params: map[string]any{"$ref": "$.metadata.enums.mode"}})
	if len(rows) != 1 || rows[0].Value != "variable" {
		t.Errorf("enum render: got %q", rows[0].Value)
	}
}

func TestInterpolatedIntent(t *testing.T) {
	data := ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{
			{Name: "to", Value: addrValue("0xabc0000000000000000000000000000000000123")},
			{Name: "value", Value: intValue(big.NewInt(1_000_000))},
			{Name: "token", Value: addrValue("0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48")},
		},
	}
	r := &Resolver{Data: data, Container: Container{ChainID: 1}}
	f := &Formatter{Resolver: r, Helpers: stubHelpers{}}
	fields := []Field{
		{Path: "value", Format: "tokenAmount", Params: map[string]any{"tokenPath": "token"}},
		{Path: "to", Format: "addressName"},
	}
	got := interpolate("Send {value} to {to}", fields, f)
	want := "Send 1 USDC to 0xabc0000000000000000000000000000000000123"
	if got != want {
		t.Errorf("interp: got %q want %q", got, want)
	}
}
