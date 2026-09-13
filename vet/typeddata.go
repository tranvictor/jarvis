package vet

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/vet/ai"
)

// TypedRequest is an eth_signTypedData_v4 payload after parse.
type TypedRequest struct {
	Mode           Mode
	ChainID        uint64
	NetworkName    string
	NetworkChainID uint64
	PrimaryType    string
	Verifying      string
	DomainChainID  *big.Int
	Message        apitypes.TypedDataMessage
	Book           []BookAddr
	Chain          Lookup
	AI             Completer
}

func measureTypedData(req TypedRequest) []Finding {
	var out []Finding
	if req.DomainChainID != nil && req.NetworkChainID != 0 && req.DomainChainID.Uint64() != req.NetworkChainID {
		out = append(out, Finding{
			Code: CodeTypedChainID,
			Risk: RiskDanger,
			Text: fmt.Sprintf("typed-data chainId is %s, jarvis is on %s (%d)", req.DomainChainID.String(), req.NetworkName, req.NetworkChainID),
		})
	}
	kind := classifyPermit(req)
	if kind == "" {
		return append(out, measureTypedPoison(req)...)
	}
	spender := typedAddr(req.Message, "spender")
	token := typedAddr(req.Message, "token")
	if token == "" {
		token = req.Verifying
	}
	spenderText := "an unspecified spender"
	if spender != "" {
		spenderText = common.HexToAddress(spender).Hex()
	}
	tokenText := "token"
	if token != "" && common.IsHexAddress(token) {
		tokenText = "token " + common.HexToAddress(token).Hex()
	}
	switch kind {
	case "permit2_pull":
		out = append(out, Finding{
			Code: CodeTypedPermit,
			Risk: RiskDanger,
			Text: fmt.Sprintf("typed-data Permit2: %s can pull tokens", spenderText),
		})
	case "dai_unlimited":
		out = append(out, Finding{
			Code: CodeTypedPermit,
			Risk: RiskDanger,
			Text: fmt.Sprintf("typed-data Permit: unlimited allowance to spender %s on %s", spenderText, tokenText),
		})
	case "eip2612_unlimited":
		out = append(out, Finding{
			Code: CodeTypedPermit,
			Risk: RiskDanger,
			Text: fmt.Sprintf("typed-data Permit: unlimited allowance to spender %s on %s", spenderText, tokenText),
		})
	case "eip2612":
		out = append(out, Finding{
			Code: CodeTypedPermit,
			Risk: RiskCaution,
			Text: fmt.Sprintf("typed-data Permit: spender %s may take tokens of %s", spenderText, tokenText),
		})
	}
	if spender != "" && !bookHas(req.Book, spender) {
		out = append(out, Finding{
			Code: CodeTypedPermit,
			Risk: RiskCaution,
			Text: fmt.Sprintf("typed-data spender %s is not in your address book", common.HexToAddress(spender).Hex()),
		})
	}
	out = append(out, measureTypedPoison(req)...)
	return out
}

func classifyPermit(req TypedRequest) string {
	pt := req.PrimaryType
	switch pt {
	case "PermitTransferFrom", "PermitWitnessTransferFrom", "PermitBatchTransferFrom", "PermitSingle", "PermitBatch":
		return "permit2_pull"
	case "Permit":
		if _, ok := req.Message["allowed"]; ok {
			if isTruthy(req.Message["allowed"]) {
				return "dai_unlimited"
			}
			return "eip2612"
		}
		if isMaxOrMissing(req.Message["value"]) {
			return "eip2612_unlimited"
		}
		return "eip2612"
	}
	return ""
}

func measureTypedPoison(req TypedRequest) []Finding {
	var addrs []jarviscommon.Address
	if req.Verifying != "" {
		addrs = append(addrs, jarviscommon.Address{Address: req.Verifying})
	}
	for _, key := range []string{"spender", "token", "owner", "holder", "operator"} {
		if a := typedAddr(req.Message, key); a != "" {
			addrs = append(addrs, jarviscommon.Address{Address: a})
		}
	}
	fake := Request{Book: req.Book, Call: &jarviscommon.FunctionCall{}}
	for _, a := range addrs {
		fake.Call.Params = append(fake.Call.Params, jarviscommon.ParamResult{
			Name: "addr", Type: "address",
			Values: []jarviscommon.Value{{Raw: a.Address, Kind: jarviscommon.DisplayAddress, Address: &a}},
		})
	}
	return measurePoison(fake)
}

func typedAddr(msg apitypes.TypedDataMessage, key string) string {
	if msg == nil {
		return ""
	}
	v, ok := msg[key]
	if !ok {
		return ""
	}
	s := fmt.Sprint(v)
	if common.IsHexAddress(s) {
		return common.HexToAddress(s).Hex()
	}
	return ""
}

func isTruthy(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1"
	default:
		s := fmt.Sprint(v)
		return s == "true" || s == "1"
	}
}

func isMaxOrMissing(v interface{}) bool {
	if v == nil {
		return true
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" {
		return true
	}
	n, ok := new(big.Int).SetString(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), 0)
	if !ok {
		n, ok = new(big.Int).SetString(s, 10)
	}
	if !ok {
		return false
	}
	return n.Cmp(maxUint256) == 0
}

func bookHas(book []BookAddr, hex string) bool {
	for _, b := range book {
		if sameAddr(b.Hex, hex) && strings.TrimSpace(b.Label) != "" && b.Label != "unknown" {
			return true
		}
	}
	return false
}

func typedPayload(req TypedRequest, src Source, codes []string) (ai.Payload, bool) {
	p := ai.Payload{
		ChainID:     req.ChainID,
		PrimaryType: "",
		LocalCodes:  append([]string(nil), codes...),
		Source:      truncateSource(src.Code),
	}
	if ai.ABIIdent(req.PrimaryType) {
		p.PrimaryType = req.PrimaryType
	}
	if req.Verifying != "" {
		h, ok := ai.ParseHexAddr(req.Verifying)
		if !ok {
			return ai.Payload{}, false
		}
		p.To = &h
	}
	if req.Message != nil {
		for k, v := range req.Message {
			name := ""
			if ai.ABIIdent(k) {
				name = k
			}
			s := strings.TrimSpace(fmt.Sprint(v))
			if !ai.ClassifiedValue(s) {
				continue
			}
			if h, ok := ai.ParseHexAddr(s); ok {
				s = h.String()
			}
			p.TypedMessage = append(p.TypedMessage, ai.Param{Name: name, Value: s})
		}
	}
	return p, true
}
