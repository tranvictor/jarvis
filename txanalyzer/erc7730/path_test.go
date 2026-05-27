package erc7730

import (
	"math/big"
	"reflect"
	"testing"
)

func TestParsePath(t *testing.T) {
	cases := []struct {
		in       string
		wantRoot PathRoot
		wantStr  string
	}{
		{"#.params.amountIn", RootStruct, "#.params.amountIn"},
		{"$.metadata.enums.interestRateMode", RootDesc, "$.metadata.enums.interestRateMode"},
		{"@.value", RootContainer, "@.value"},
		{"to", RootStruct, "#.to"},
		{"recipients.[0]", RootStruct, "#.recipients.[0]"},
		{"recipients[0]", RootStruct, "#.recipients.[0]"},
		{"path.[0:20]", RootStruct, "#.path.[0:20]"},
		{"pools.[-1]", RootStruct, "#.pools.[-1]"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			p, err := ParsePath(c.in)
			if err != nil {
				t.Fatalf("parse %q: %v", c.in, err)
			}
			if p.Root != c.wantRoot {
				t.Errorf("root: got %d want %d", p.Root, c.wantRoot)
			}
			if got := p.String(); got != c.wantStr {
				t.Errorf("string: got %q want %q", got, c.wantStr)
			}
		})
	}
}

func TestResolverFieldAndContainer(t *testing.T) {
	data := ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{
			{Name: "to", Value: addrValue("0xabc0000000000000000000000000000000000123")},
			{Name: "amount", Value: intValue(big.NewInt(1000000))},
		},
	}
	r := &Resolver{
		Data:      data,
		Container: Container{To: "0xfff", ChainID: 1, Value: big.NewInt(5)},
	}
	tests := []struct {
		path     string
		wantKind ResolvedKind
		want     interface{}
	}{
		{"to", ResolvedAddress, "0xabc0000000000000000000000000000000000123"},
		{"#.amount", ResolvedInt, int64(1000000)},
		{"@.chainId", ResolvedInt, int64(1)},
		{"@.value", ResolvedInt, int64(5)},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			p, err := ParsePath(tc.path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			v, err := r.Resolve(p)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if v.Kind != tc.wantKind {
				t.Fatalf("kind: got %d want %d", v.Kind, tc.wantKind)
			}
			switch tc.wantKind {
			case ResolvedInt:
				if v.Int.Int64() != tc.want.(int64) {
					t.Errorf("int: got %d want %d", v.Int.Int64(), tc.want)
				}
			case ResolvedAddress:
				if v.Addr != tc.want.(string) {
					t.Errorf("addr: got %q want %q", v.Addr, tc.want)
				}
			}
		})
	}
}

func TestResolverSliceOnBytes(t *testing.T) {
	bytes := []byte{0xA0, 0xb8, 0x69, 0x91, 0xc6, 0x21, 0x8b, 0x36, 0xc1, 0xd1,
		0x9d, 0x4a, 0x2e, 0x9e, 0xb0, 0xce, 0x36, 0x06, 0xeb, 0x48,
		0xde, 0xad, 0xbe, 0xef}
	data := ResolvedValue{
		Kind: ResolvedTuple,
		Tuple: []ResolvedField{{Name: "path", Value: bytesValue(bytes)}},
	}
	r := &Resolver{Data: data}
	p, _ := ParsePath("#.path.[0:20]")
	v, err := r.Resolve(p)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := bytes[:20]
	if !reflect.DeepEqual(v.Bytes, want) {
		t.Errorf("slice: got %x want %x", v.Bytes, want)
	}
}

func TestResolverNegativeIndex(t *testing.T) {
	arr := []ResolvedValue{intValue(big.NewInt(10)), intValue(big.NewInt(20)), intValue(big.NewInt(30))}
	data := ResolvedValue{Kind: ResolvedTuple, Tuple: []ResolvedField{{Name: "pools", Value: ResolvedValue{Kind: ResolvedArray, Array: arr}}}}
	r := &Resolver{Data: data}
	p, _ := ParsePath("pools.[-1]")
	v, err := r.Resolve(p)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if v.Int.Int64() != 30 {
		t.Errorf("last: got %d want 30", v.Int.Int64())
	}
}
