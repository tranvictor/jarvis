package erc7730

import (
	"math/big"
	"testing"
)

func TestApplyParamNamesOverridesABIInternalNames(t *testing.T) {
	two := new(big.Int).Mul(big.NewInt(2), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	data := ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{{
			Name: "_execution",
			Value: ResolvedValue{
				Kind: ResolvedTuple,
				Tuple: []ResolvedField{{
					Name: "desc",
					Value: ResolvedValue{
						Kind: ResolvedTuple,
						Tuple: []ResolvedField{
							{Name: "srcToken", Value: addrValue("0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")},
							{Name: "amount", Value: intValue(two)},
						},
					},
				}},
			},
		}},
	}

	renamed := applyParamNames(data, []string{"execution"})
	r := &Resolver{Data: renamed}
	p, err := ParsePath("execution.desc.srcToken")
	if err != nil {
		t.Fatal(err)
	}
	v, err := r.Resolve(p)
	if err != nil {
		t.Fatalf("resolve srcToken: %v", err)
	}
	if v.Addr != "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		t.Fatalf("srcToken: got %q", v.Addr)
	}
	p, _ = ParsePath("execution.desc.amount")
	v, err = r.Resolve(p)
	if err != nil {
		t.Fatalf("resolve amount: %v", err)
	}
	if v.Int.Cmp(two) != 0 {
		t.Fatalf("amount: got %s want %s", v.Int, two)
	}
}
