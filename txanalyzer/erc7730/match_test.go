package erc7730

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// staticSource is an in-memory DescriptorSource for tests. Production
// uses LocalRegistry; here we just supply a slice and let both
// FindByContract and FindByEIP712 return every candidate.
type staticSource []*Descriptor

func (s staticSource) FindByContract(chainID uint64, address string) []*Descriptor {
	return s
}

func (s staticSource) FindByEIP712(td *apitypes.TypedData) []*Descriptor {
	return s
}

func TestParseFormatKey(t *testing.T) {
	cases := []struct {
		key        string
		wantName   string
		wantNames  []string
		wantTypes  []string
		wantSelHex string
	}{
		{
			key:        "transfer(address to,uint256 value)",
			wantName:   "transfer",
			wantNames:  []string{"to", "value"},
			wantTypes:  []string{"address", "uint256"},
			wantSelHex: "a9059cbb",
		},
		{
			key:        "approve(address spender,uint256 value)",
			wantName:   "approve",
			wantNames:  []string{"spender", "value"},
			wantTypes:  []string{"address", "uint256"},
			wantSelHex: "095ea7b3",
		},
		{
			key:        "submitOrder((address token,uint256 amount,uint256 price) order,bytes32 salt)",
			wantName:   "submitOrder",
			wantNames:  []string{"order", "salt"},
			wantTypes:  []string{"(address,uint256,uint256)", "bytes32"},
			wantSelHex: "", // computed dynamically below
		},
		{
			key:        "airdrop(address[] recipients,uint256[3] values)",
			wantName:   "airdrop",
			wantNames:  []string{"recipients", "values"},
			wantTypes:  []string{"address[]", "uint256[3]"},
			wantSelHex: "",
		},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			got, err := parseFormatKey(c.key)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got.Name != c.wantName {
				t.Errorf("name: got %q want %q", got.Name, c.wantName)
			}
			if !equalStrings(got.ParamNames, c.wantNames) {
				t.Errorf("names: got %v want %v", got.ParamNames, c.wantNames)
			}
			if !equalStrings(got.ParamTypes, c.wantTypes) {
				t.Errorf("types: got %v want %v", got.ParamTypes, c.wantTypes)
			}
			if c.wantSelHex != "" {
				if hex.EncodeToString(got.Selector) != c.wantSelHex {
					t.Errorf("selector: got %s want %s",
						hex.EncodeToString(got.Selector), c.wantSelHex)
				}
			}
		})
	}
}

func TestFindContractMatchSelector(t *testing.T) {
	desc := &Descriptor{
		Context: Context{Contract: &ContractCtx{Deployments: []Deployment{{ChainID: 1, Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7"}}}},
		Display: Display{Formats: map[string]*Format{
			"transfer(address to,uint256 value)": {Intent: Intent{Text: "Send"}},
			"approve(address spender,uint256 value)": {Intent: Intent{Text: "Approve"}},
		}},
	}
	src := staticSource{desc}

	// 0xa9059cbb is the selector for transfer(address,uint256).
	calldata, _ := hex.DecodeString("a9059cbb" + zeroPad("e", 64) + zeroPad("64", 64))
	m := FindContractMatch(src, ContractMatchInput{ChainID: 1, To: "0xdac17f958d2ee523a2206206994597c13d831ec7", Calldata: calldata})
	if m == nil {
		t.Fatalf("expected match for transfer selector")
	}
	if m.FormatKey != "transfer(address to,uint256 value)" {
		t.Errorf("wrong format key: %s", m.FormatKey)
	}
}

func TestFindContractMatchUnknownSelector(t *testing.T) {
	desc := &Descriptor{
		Display: Display{Formats: map[string]*Format{
			"transfer(address to,uint256 value)": {},
		}},
	}
	src := staticSource{desc}
	calldata := []byte{0xff, 0xff, 0xff, 0xff, 0x00}
	m := FindContractMatch(src, ContractMatchInput{ChainID: 1, To: "0x0", Calldata: calldata})
	if m != nil {
		t.Errorf("expected no match for unknown selector, got %v", m.FormatKey)
	}
}

func TestEIP712MatchByDomainSeparator(t *testing.T) {
	td := &apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Greeting": []apitypes.Type{{Name: "content", Type: "string"}},
		},
		PrimaryType: "Greeting",
		Domain: apitypes.TypedDataDomain{
			Name:              "Permit2",
			VerifyingContract: "0x000000000022D473030F116dDEE9F6B43aC78BA3",
		},
		Message: map[string]any{"content": "hi"},
	}
	sep, err := td.HashStruct("EIP712Domain", td.Domain.Map())
	if err != nil {
		t.Fatalf("hash domain: %v", err)
	}
	sepHex := "0x" + hex.EncodeToString(sep)

	desc := &Descriptor{
		Context: Context{EIP712: &EIP712Ctx{DomainSeparator: sepHex}},
		Display: Display{Formats: map[string]*Format{
			"Greeting(string content)": {Intent: Intent{Text: "Greet"}},
		}},
	}
	m := FindEIP712Match(staticSource{desc}, EIP712MatchInput{TypedData: td})
	if m == nil {
		t.Fatalf("expected match by domain separator")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func zeroPad(suffixHex string, width int) string {
	pad := width - len(suffixHex)
	if pad < 0 {
		return suffixHex[:width]
	}
	var b bytes.Buffer
	for i := 0; i < pad; i++ {
		b.WriteByte('0')
	}
	b.WriteString(suffixHex)
	return b.String()
}
