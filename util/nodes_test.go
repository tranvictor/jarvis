package util

import "testing"

func TestFindNodeNameByURL(t *testing.T) {
	nodes := map[string]string{
		"mainnet-kyber": "https://ethereum-rpc.kyberswap.com/",
		"own":           "https://my.example/rpc",
		"infra":         "rpc-mainnet.monadinfra.com/rpc/KEY",
	}
	name, ok := FindNodeNameByURL(nodes, "https://ethereum-rpc.kyberswap.com")
	if !ok || name != "mainnet-kyber" {
		t.Fatalf("got (%q, %v), want mainnet-kyber", name, ok)
	}
	if _, ok := FindNodeNameByURL(nodes, "https://other.example"); ok {
		t.Fatal("expected no match")
	}
	name, ok = FindNodeNameByURL(nodes, "https://rpc-mainnet.monadinfra.com/rpc/KEY")
	if !ok || name != "infra" {
		t.Fatalf("schemeless stored URL: got (%q, %v), want infra", name, ok)
	}
	name, ok = FindNodeNameByURL(map[string]string{"infra": "https://rpc-mainnet.monadinfra.com/rpc/KEY"}, "rpc-mainnet.monadinfra.com/rpc/KEY")
	if !ok || name != "infra" {
		t.Fatalf("schemeless lookup: got (%q, %v), want infra", name, ok)
	}
}
