package vet

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/vet/ai"
)

func TestToPayloadStripsDescCanary(t *testing.T) {
	const canary = "CANARY_UncleBob_7f3a"
	fc := &jarviscommon.FunctionCall{
		Destination: addr(testUSDC, canary),
		Method:      "transfer",
		Value:       big.NewInt(0),
		Params: []jarviscommon.ParamResult{
			addrP("to", testMe, canary),
			uintP("amount", "1000"),
		},
	}
	p, ok := ToPayload(Request{
		ChainID: 1,
		To:      addr(testUSDC, canary),
		Call:    fc,
	}, Source{Code: "contract Token { function transfer(address to, uint256 amount) public {} }"}, []string{"admin"})
	if !ok {
		t.Fatal("ToPayload rejected a classified call")
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), canary) {
		t.Fatalf("canary leaked into payload: %s", raw)
	}
	if strings.Contains(string(raw), "USDC") {
		t.Fatalf("label leaked: %s", raw)
	}
	if p.Method != "transfer" || p.To == nil {
		t.Fatalf("payload incomplete: %+v", p)
	}
}

func TestAIPackageImports(t *testing.T) {
	fset := token.NewFileSet()
	dir := filepath.Join("ai")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"addrbook", "github.com/tranvictor/jarvis/db", "github.com/tranvictor/jarvis/accounts"}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, e.Name(), src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if strings.Contains(path, bad) {
					t.Errorf("%s imports %s", e.Name(), path)
				}
			}
		}
	}
}

func TestVetProductionHasNoDescSelector(t *testing.T) {
	fset := token.NewFileSet()
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && (info.Name() == "ai" || strings.HasSuffix(info.Name(), "_test.go")) {
			if info.Name() == "ai" {
				return filepath.SkipDir
			}
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "Desc" {
				t.Errorf("%s: production vet code must not read .Desc", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassifiedConstructors(t *testing.T) {
	if _, ok := ai.ParseHexAddr("Mom"); ok {
		t.Fatal("name must not parse as hex addr")
	}
	if ai.ABIIdent("Uncle Bob") {
		t.Fatal("spaces are not abi idents")
	}
	if !ai.ABIIdent("transferOwnership") {
		t.Fatal("method name")
	}
}
