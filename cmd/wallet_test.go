package cmd

import (
	"reflect"
	"testing"

	"github.com/tranvictor/jarvis/accounts/types"
)

func TestHwChooseOptionsFirstPage(t *testing.T) {
	page := []*types.AccDesc{
		{Address: "0xaaa", Derpath: "m/44'/60'/0'/0"},
		{Address: "0xbbb", Derpath: "m/44'/60'/0'/1"},
	}
	labels, next, back, custom := hwChooseOptions(page, 0)
	want := []string{
		"0xaaa  m/44'/60'/0'/0",
		"0xbbb  m/44'/60'/0'/1",
		"next page",
		"custom derivation path",
	}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	if next != 2 || back != -1 || custom != 3 {
		t.Fatalf("indices next=%d back=%d custom=%d", next, back, custom)
	}
}

func TestHwChooseOptionsLaterPageHasPrevious(t *testing.T) {
	page := []*types.AccDesc{{Address: "0xccc", Derpath: "m/44'/60'/0'/5"}}
	labels, next, back, custom := hwChooseOptions(page, 1)
	want := []string{
		"0xccc  m/44'/60'/0'/5",
		"next page",
		"previous page",
		"custom derivation path",
	}
	if !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	if next != 1 || back != 2 || custom != 3 {
		t.Fatalf("indices next=%d back=%d custom=%d", next, back, custom)
	}
}
