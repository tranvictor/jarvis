package explorers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFlattenSourceJSONInput(t *testing.T) {
	raw := `{{"language":"Solidity","sources":{"A.sol":{"content":"pragma solidity ^0.8;"}}}}`
	got := flattenSource(raw)
	if !strings.Contains(got, "file: A.sol") || !strings.Contains(got, "pragma solidity") {
		t.Fatalf("flatten: %q", got)
	}
}

func TestGetVerifiedSourceEtherscan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"1","message":"OK","result":[{"ContractName":"Foo","ABI":"[]","Proxy":"0","Implementation":"","SourceCode":"contract Foo {}"}]}`))
	}))
	t.Cleanup(srv.Close)
	ee := NewEtherscanLikeExplorer(srv.URL, "k", 1)
	src, err := ee.GetVerifiedSource("0x0000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if !src.Verified || !strings.Contains(src.Source, "contract Foo") {
		t.Fatalf("%+v", src)
	}
}
