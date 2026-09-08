package util

import (
	"math/big"
	"strings"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

// PaymentReading is the one-line form of a native send or an ERC-20
// transfer/transferFrom. Signing cards and the Call tree use it so "the Safe
// is sending 1.5 ETH to Alice" reads as a send, not as a contract call.
type PaymentReading struct {
	Amount string
	To     ui.StyledText
	From   ui.StyledText // transferFrom only
}

func nativePayment(to jarviscommon.Address, value *big.Int, network networks.Network) *PaymentReading {
	if value == nil || value.Sign() <= 0 {
		return nil
	}
	dec, sym := uint64(18), "ETH"
	if network != nil {
		dec = network.GetNativeTokenDecimal()
		sym = network.GetNativeTokenSymbol()
	}
	amount := jarviscommon.CompactAmount(jarviscommon.BigToFloatString(value, dec)) + " " + sym
	return &PaymentReading{Amount: amount, To: StyledAddress(to)}
}

func tokenPayment(fc *jarviscommon.FunctionCall) *PaymentReading {
	if fc == nil {
		return nil
	}
	switch fc.Method {
	case "transfer":
		to := namedAddress(fc, "to", "dst", "recipient")
		amount := namedValue(fc, "value", "amount", "wad")
		if to == nil {
			to = addressAt(fc, 0)
		}
		if amount == nil {
			amount = valueAt(fc, 1)
		}
		if to == nil || amount == nil {
			return nil
		}
		return &PaymentReading{
			Amount: paymentAmountText(*amount, fc.Destination),
			To:     styledParamAddress(*to),
		}
	case "transferFrom":
		from := namedAddress(fc, "from", "src")
		to := namedAddress(fc, "to", "dst", "recipient")
		amount := namedValue(fc, "value", "amount", "wad")
		if from == nil {
			from = addressAt(fc, 0)
		}
		if to == nil {
			to = addressAt(fc, 1)
		}
		if amount == nil {
			amount = valueAt(fc, 2)
		}
		if to == nil || amount == nil {
			return nil
		}
		p := &PaymentReading{
			Amount: paymentAmountText(*amount, fc.Destination),
			To:     styledParamAddress(*to),
		}
		if from != nil {
			p.From = styledParamAddress(*from)
		}
		return p
	default:
		return nil
	}
}

func paymentAmountText(v jarviscommon.Value, dest jarviscommon.Address) string {
	symbol := paymentTokenSymbol(v, dest)
	decimals := uint64(0)
	if v.Token != nil {
		decimals = v.Token.Decimal
	} else if dest.Decimal > 0 {
		decimals = uint64(dest.Decimal)
	}
	if decimals > 0 {
		human := jarviscommon.CompactAmount(jarviscommon.BigToFloatString(jarviscommon.StringToBig(v.Raw), decimals))
		if symbol != "" {
			return human + " " + symbol
		}
		return human
	}
	return jarviscommon.ReadableNumber(v.Raw)
}

func paymentTokenSymbol(v jarviscommon.Value, dest jarviscommon.Address) string {
	if v.Token != nil && v.Token.Symbol != "" {
		return v.Token.Symbol
	}
	d := strings.TrimSpace(dest.Desc)
	d = strings.TrimSuffix(d, " token")
	d = strings.TrimSpace(d)
	if d != "" && d != "unknown" {
		return d
	}
	return ""
}

func namedAddress(fc *jarviscommon.FunctionCall, names ...string) *jarviscommon.Address {
	if p := namedParam(fc, names...); p != nil {
		return paramAddress(p)
	}
	return nil
}

func namedValue(fc *jarviscommon.FunctionCall, names ...string) *jarviscommon.Value {
	if p := namedParam(fc, names...); p != nil {
		return paramScalar(p)
	}
	return nil
}

func namedParam(fc *jarviscommon.FunctionCall, names ...string) *jarviscommon.ParamResult {
	want := map[string]bool{}
	for _, n := range names {
		want[canonicalParamName(n)] = true
	}
	for i := range fc.Params {
		if want[canonicalParamName(fc.Params[i].Name)] {
			return &fc.Params[i]
		}
	}
	return nil
}

func canonicalParamName(name string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "_"))
}

func addressAt(fc *jarviscommon.FunctionCall, i int) *jarviscommon.Address {
	if i < 0 || i >= len(fc.Params) {
		return nil
	}
	return paramAddress(&fc.Params[i])
}

func valueAt(fc *jarviscommon.FunctionCall, i int) *jarviscommon.Value {
	if i < 0 || i >= len(fc.Params) {
		return nil
	}
	return paramScalar(&fc.Params[i])
}

func paramAddress(p *jarviscommon.ParamResult) *jarviscommon.Address {
	if p == nil || len(p.Values) != 1 || p.Values[0].Address == nil {
		return nil
	}
	return p.Values[0].Address
}

func paramScalar(p *jarviscommon.ParamResult) *jarviscommon.Value {
	if p == nil || len(p.Values) != 1 {
		return nil
	}
	return &p.Values[0]
}
