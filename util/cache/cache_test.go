package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSetCacheDoesNotWriteUntilFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	resetForTest(path)

	if err := SetCache("foo", "bar"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("SetCache must not rewrite the file, stat err=%v", err)
	}
	if got, found := GetCache("foo"); !found || got != "bar" {
		t.Fatalf("GetCache before Flush: %q %v", got, found)
	}

	if err := Flush(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var onDisk simpleCache
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	if onDisk.Data["foo"] != "bar" {
		t.Fatalf("disk cache: %+v", onDisk.Data)
	}
}
