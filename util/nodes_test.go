package util

import "testing"

func TestFindNodeNameByURL(t *testing.T) {
	nodes := map[string]string{
		"mainnet-kyber": "https://ethereum-rpc.kyberswap.com/",
		"own":           "https://my.example/rpc",
	}
	name, ok := FindNodeNameByURL(nodes, "https://ethereum-rpc.kyberswap.com")
	if !ok || name != "mainnet-kyber" {
		t.Fatalf("got (%q, %v), want mainnet-kyber", name, ok)
	}
	if _, ok := FindNodeNameByURL(nodes, "https://other.example"); ok {
		t.Fatal("expected no match")
	}
}
