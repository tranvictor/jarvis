package vet

import (
	"bytes"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/db"
)

const (
	CodeDelegateCall        = "delegatecall"
	CodeUnverified          = "unverified"
	CodeProxyImplUnverified = "proxy_impl_unverified"
	CodeCreate              = "create"
	CodeAdmin               = "admin"
	CodeUpgrade             = "upgrade"
	CodePermit2             = "permit2"
	CodeDrain               = "drain"
	CodePoison              = "poison"
	CodeTokenRecipient      = "token_recipient"
	CodeMinOutZero          = "min_out_zero"
	CodeTypedPermit         = "typed_permit"
	CodeTypedChainID        = "typed_chainid"
	CodeAISkip              = "ai_skip"
	CodeGrok                = "grok"
)

var adminMethods = map[string]struct{}{
	"transferOwnership": {},
	"renounceOwnership": {},
	"changeOwner":       {},
	"setOwner":          {},
	"setAdmin":          {},
	"grantRole":         {},
	"grantRoles":        {},
	"setAuthority":      {},
	"acceptOwnership":   {},
}

var upgradeMethods = map[string]struct{}{
	"upgradeTo":            {},
	"upgradeToAndCall":     {},
	"changeImplementation": {},
	"setImplementation":    {},
	"diamondCut":           {},
}

var drainMethods = map[string]struct{}{
	"sweep":             {},
	"sweepToken":        {},
	"sweepTokens":       {},
	"rescue":            {},
	"rescueTokens":      {},
	"rescueEther":       {},
	"emergencyWithdraw": {},
	"emergencyExit":     {},
	"skim":              {},
}

var permit2Methods = map[string]struct{}{
	"permitTransferFrom":        {},
	"permitWitnessTransferFrom": {},
	"permitBatchTransferFrom":   {},
}

var minOutNames = map[string]struct{}{
	"amountoutmin":     {},
	"minamountout":     {},
	"minreturn":        {},
	"minreturnamount":  {},
	"amountoutminimum": {},
}

// Canonical Permit2 (CREATE2) address used on most EVM chains.
var permit2Addr = common.HexToAddress("0x000000000022D473030F116dDEE9F6B43aC78BA3")

var maxUint256, _ = new(big.Int).SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)

func measureDelegateCall(req Request) []Finding {
	if !req.DelegateCall {
		return nil
	}
	if req.MultiSend {
		return []Finding{{
			Code: CodeDelegateCall,
			Risk: RiskDanger,
			Text: "DELEGATECALL into MultiSend: every inner call below runs with the Safe's full authority",
		}}
	}
	return []Finding{{
		Code: CodeDelegateCall,
		Risk: RiskDanger,
		Text: "DELEGATECALL: the target's code runs in the Safe's own context",
	}}
}

func measureCreate(req Request) []Finding {
	if !req.Create {
		return nil
	}
	return []Finding{{
		Code: CodeCreate,
		Risk: RiskCaution,
		Text: "this transaction deploys a new contract; source review is skipped",
	}}
}

func measureUnverified(req Request) []Finding {
	if req.Create || req.Chain == nil {
		return nil
	}
	dest := effectiveDest(req)
	if dest == "" || !common.IsHexAddress(dest) {
		return nil
	}
	src, err := req.Chain.Source(dest)
	if err != nil {
		return []Finding{{
			Code: CodeAISkip,
			Risk: RiskCaution,
			Text: fmt.Sprintf("vet skipped: could not fetch source for %s (%s)", common.HexToAddress(dest).Hex(), err),
		}}
	}
	impl := strings.TrimSpace(src.Implementation)
	if impl == "" && req.Chain != nil {
		if got, ierr := req.Chain.Implementation(dest); ierr == nil {
			impl = got
		}
	}
	var out []Finding
	if !src.Verified || src.Code == "" {
		out = append(out, Finding{
			Code: CodeUnverified,
			Risk: RiskDanger,
			Text: fmt.Sprintf("%s has no verified source; vet cannot read the code that will run", common.HexToAddress(dest).Hex()),
		})
	}
	if impl != "" && !sameAddr(impl, dest) {
		implSrc, err := req.Chain.Source(impl)
		if err != nil {
			out = append(out, Finding{
				Code: CodeAISkip,
				Risk: RiskCaution,
				Text: fmt.Sprintf("vet skipped: could not resolve implementation %s (%s)", common.HexToAddress(impl).Hex(), err),
			})
			return out
		}
		if !implSrc.Verified || implSrc.Code == "" {
			out = append(out, Finding{
				Code: CodeProxyImplUnverified,
				Risk: RiskDanger,
				Text: fmt.Sprintf("proxy %s delegates to unverified implementation %s", common.HexToAddress(dest).Hex(), common.HexToAddress(impl).Hex()),
			})
		}
	}
	return out
}

func measureMethods(req Request) []Finding {
	var out []Finding
	walkCalls(req.Call, func(fc *jarviscommon.FunctionCall) {
		if fc.Method == "" {
			return
		}
		dest := fc.Destination.Address
		if dest == "" {
			dest = req.To.Address
		}
		destHex := dest
		if common.IsHexAddress(dest) {
			destHex = common.HexToAddress(dest).Hex()
		}
		if _, ok := adminMethods[fc.Method]; ok {
			risk := RiskDanger
			if fc.Method == "acceptOwnership" {
				risk = RiskCaution
			}
			out = append(out, Finding{
				Code: CodeAdmin,
				Risk: risk,
				Text: fmt.Sprintf("calls %s on %s", fc.Method, destHex),
			})
		}
		if _, ok := upgradeMethods[fc.Method]; ok {
			out = append(out, Finding{
				Code: CodeUpgrade,
				Risk: RiskDanger,
				Text: fmt.Sprintf("calls %s on %s", fc.Method, destHex),
			})
		}
		if _, ok := drainMethods[fc.Method]; ok {
			out = append(out, Finding{
				Code: CodeDrain,
				Risk: RiskDanger,
				Text: fmt.Sprintf("calls %s on %s", fc.Method, destHex),
			})
		}
		if _, ok := permit2Methods[fc.Method]; ok {
			spender := paramAddress(fc, "spender", "operator")
			spenderText := "an unspecified spender"
			if spender != nil && spender.Address != "" {
				spenderText = common.HexToAddress(spender.Address).Hex()
			}
			out = append(out, Finding{
				Code: CodePermit2,
				Risk: RiskDanger,
				Text: fmt.Sprintf("Permit2 %s: spender %s can pull tokens", fc.Method, spenderText),
			})
		}
		if fc.Method == "permit" {
			if allowed := paramValue(fc, "allowed"); allowed != nil && allowed.Raw == "true" {
				spender := paramAddress(fc, "spender")
				spenderText := "an unspecified spender"
				if spender != nil && spender.Address != "" {
					spenderText = common.HexToAddress(spender.Address).Hex()
				}
				out = append(out, Finding{
					Code: CodePermit2,
					Risk: RiskDanger,
					Text: fmt.Sprintf("DAI-style permit sets allowed=true for spender %s", spenderText),
				})
			}
		}
		if sameAddr(dest, permit2Addr.Hex()) && (fc.Method == "approve" || fc.Method == "permit") {
			spender := paramAddress(fc, "spender")
			spenderText := "an unspecified spender"
			if spender != nil && spender.Address != "" {
				spenderText = common.HexToAddress(spender.Address).Hex()
			}
			out = append(out, Finding{
				Code: CodePermit2,
				Risk: RiskDanger,
				Text: fmt.Sprintf("Permit2 %s: spender %s can pull tokens", fc.Method, spenderText),
			})
		}
		if fc.Method == "transfer" || fc.Method == "transferFrom" || fc.Method == "safeTransferFrom" {
			to := paramAddress(fc, "to", "recipient", "dst")
			if to != nil && !jarviscommon.IsKnownAddress(*to) {
				out = append(out, Finding{
					Code: CodeTokenRecipient,
					Risk: RiskCaution,
					Text: fmt.Sprintf("token recipient %s is not in your address book", common.HexToAddress(to.Address).Hex()),
				})
			}
		}
		for i := range fc.Params {
			p := &fc.Params[i]
			if _, ok := minOutNames[strings.ToLower(p.Name)]; !ok {
				continue
			}
			if len(p.Values) != 1 {
				continue
			}
			if isZeroAmount(p.Values[0].Raw) {
				out = append(out, Finding{
					Code: CodeMinOutZero,
					Risk: RiskDanger,
					Text: "swap accepts a minimum out of 0 (no slippage protection)",
				})
			}
		}
	})
	return out
}

func measurePoison(req Request) []Finding {
	book := req.Book
	if book == nil {
		for hex, label := range db.AllAddresses() {
			book = append(book, BookAddr{Hex: hex, Label: label})
		}
	}
	if len(book) == 0 {
		return nil
	}
	var addrs []jarviscommon.Address
	if req.To.Address != "" {
		addrs = append(addrs, req.To)
	}
	walkAddresses(req.Call, func(a jarviscommon.Address) {
		addrs = append(addrs, a)
	})
	var out []Finding
	seen := map[string]struct{}{}
	for _, a := range addrs {
		if !common.IsHexAddress(a.Address) {
			continue
		}
		got := common.HexToAddress(a.Address)
		for _, b := range book {
			if !common.IsHexAddress(b.Hex) {
				continue
			}
			want := common.HexToAddress(b.Hex)
			if got == want || !poisonPair(got, want) {
				continue
			}
			key := got.Hex() + "|" + want.Hex()
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			label := b.Label
			if label == "" {
				label = want.Hex()
			}
			out = append(out, Finding{
				Code: CodePoison,
				Risk: RiskDanger,
				Text: fmt.Sprintf("%s looks like your address-book entry %s (%s)", got.Hex(), want.Hex(), label),
			})
		}
	}
	return out
}

func poisonPair(a, b common.Address) bool {
	ab, bb := a.Bytes(), b.Bytes()
	return bytes.Equal(ab[:2], bb[:2]) && bytes.Equal(ab[18:], bb[18:])
}

func effectiveDest(req Request) string {
	if req.To.Address != "" {
		return req.To.Address
	}
	if req.Call != nil {
		return req.Call.Destination.Address
	}
	return ""
}

func sameAddr(a, b string) bool {
	if !common.IsHexAddress(a) || !common.IsHexAddress(b) {
		return strings.EqualFold(a, b)
	}
	return common.HexToAddress(a) == common.HexToAddress(b)
}

func isZeroAmount(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || s == "0" || s == "0x0" || s == "0x00" {
		return true
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		n, ok := new(big.Int).SetString(s[2:], 16)
		return ok && n.Sign() == 0
	}
	n, ok := new(big.Int).SetString(s, 10)
	return ok && n.Sign() == 0
}
