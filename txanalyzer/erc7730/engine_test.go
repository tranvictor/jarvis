package erc7730

import "testing"

func TestLookupEngineDisablesAutoSync(t *testing.T) {
	if DefaultEngine().AutoSyncEvery != defaultAutoSync {
		t.Fatalf("DefaultEngine AutoSyncEvery = %s", DefaultEngine().AutoSyncEvery)
	}
	if LookupEngine().AutoSyncEvery != 0 {
		t.Fatalf("LookupEngine must not block on a registry download, AutoSyncEvery=%s", LookupEngine().AutoSyncEvery)
	}
}
