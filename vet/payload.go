package vet

import (
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/vet/ai"
)

const maxSourceRunes = 200000

// ToPayload copies an allowlisted subset of Request into an [ai.Payload].
// It never reads Address.Desc, Book labels, or warning text.
func ToPayload(req Request, src Source, localCodes []string) (ai.Payload, bool) {
	p := ai.Payload{
		ChainID:    req.ChainID,
		Create:     req.Create,
		LocalCodes: append([]string(nil), localCodes...),
	}
	if req.To.Address != "" {
		h, ok := ai.ParseHexAddr(req.To.Address)
		if !ok {
			return ai.Payload{}, false
		}
		p.To = &h
	}
	if req.Value != nil && req.Value.Sign() > 0 {
		p.Value = req.Value.String()
	}
	if len(req.Data) > 0 {
		p.Data = "0x" + hex.EncodeToString(req.Data)
	}
	if req.Call != nil {
		call, ok := convertCall(req.Call)
		if !ok {
			return ai.Payload{}, false
		}
		p.Method = call.Method
		p.Params = call.Params
		p.Calls = call.Calls
		if p.To == nil {
			to := call.To
			p.To = &to
		}
	}
	if src.Code != "" {
		p.Source = truncateSource(src.Code)
	}
	return p, true
}

func convertCall(fc *jarviscommon.FunctionCall) (ai.Call, bool) {
	var out ai.Call
	if fc.Destination.Address != "" {
		h, ok := ai.ParseHexAddr(fc.Destination.Address)
		if !ok {
			return ai.Call{}, false
		}
		out.To = h
	}
	if fc.Value != nil && fc.Value.Sign() > 0 {
		out.Value = fc.Value.String()
	}
	if fc.Method != "" {
		if !ai.ABIIdent(fc.Method) {
			return ai.Call{}, false
		}
		out.Method = fc.Method
	}
	params, ok := convertParams(fc.Params)
	if !ok {
		return ai.Call{}, false
	}
	out.Params = params
	for _, inner := range fc.DecodedFunctionCalls {
		if inner == nil {
			continue
		}
		c, ok := convertCall(inner)
		if !ok {
			return ai.Call{}, false
		}
		out.Calls = append(out.Calls, c)
	}
	return out, true
}

func convertParams(ps []jarviscommon.ParamResult) ([]ai.Param, bool) {
	var out []ai.Param
	for _, p := range ps {
		name := ""
		if ai.ABIIdent(p.Name) {
			name = p.Name
		}
		typ := p.Type
		if typ != "" && !plausibleABIType(typ) {
			typ = ""
		}
		for _, v := range p.Values {
			val, ok := classifiedRaw(v)
			if !ok {
				// Free text is omitted, not forwarded. Do not fail the
				// whole payload — a string argument must not become a
				// reason to skip source review.
				continue
			}
			out = append(out, ai.Param{Name: name, Type: typ, Value: val})
		}
		for _, t := range p.Tuples {
			nested, ok := convertParams(t.Values)
			if !ok {
				return nil, false
			}
			out = append(out, nested...)
		}
		nested, ok := convertParams(p.Arrays)
		if !ok {
			return nil, false
		}
		out = append(out, nested...)
	}
	return out, true
}

func classifiedRaw(v jarviscommon.Value) (string, bool) {
	if v.Address != nil && v.Address.Address != "" {
		if _, ok := ai.ParseHexAddr(v.Address.Address); !ok {
			return "", false
		}
		return common.HexToAddress(v.Address.Address).Hex(), true
	}
	s := strings.TrimSpace(v.Raw)
	if s == "" {
		return "", false
	}
	if ai.ClassifiedValue(s) {
		if h, ok := ai.ParseHexAddr(s); ok {
			return h.String(), true
		}
		return s, true
	}
	return "", false
}

func plausibleABIType(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return false
	}
	for _, c := range t {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '[' || c == ']' || c == '(' || c == ')' || c == ',' || c == '_') {
			return false
		}
	}
	return true
}

func truncateSource(s string) string {
	if len([]rune(s)) <= maxSourceRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxSourceRunes]) + "\n// ... truncated ..."
}
