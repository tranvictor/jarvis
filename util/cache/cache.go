package cache

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	CACHE_PATH string = filepath.Join(getHomeDir(), ".jarvis", "cache.json")
	cache      *simpleCache
	dirty      bool
	mu         sync.Mutex
)

func getHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr.HomeDir
}

type simpleCache struct {
	Data map[string]string `json:"Data"`
}

func (self *simpleCache) snapshot() *simpleCache {
	cp := &simpleCache{Data: make(map[string]string, len(self.Data))}
	for k, v := range self.Data {
		cp.Data[k] = v
	}
	return cp
}

func persistTo(path string, c *simpleCache) error {
	jsonData, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, jsonData, 0644)
}

func loadSimpleCache() *simpleCache {
	if cache != nil {
		return cache
	}
	cache = &simpleCache{
		Data: map[string]string{},
	}
	content, err := os.ReadFile(CACHE_PATH)
	if err != nil {
		return cache
	}
	err = json.Unmarshal(content, cache)
	if err != nil {
		return cache
	}
	return cache
}

// Flush writes dirty cache entries to disk. SetCache only updates
// memory so a command that looks up many ABIs or contract names does
// not rewrite ~/.jarvis/cache.json on every hit. cmd.Execute defers
// Flush so a normal process exit still persists.
func Flush() error {
	mu.Lock()
	if !dirty || cache == nil {
		mu.Unlock()
		return nil
	}
	snap := cache.snapshot()
	path := CACHE_PATH
	dirty = false
	mu.Unlock()

	if err := persistTo(path, snap); err != nil {
		mu.Lock()
		dirty = true
		mu.Unlock()
		return err
	}
	return nil
}

// ResetForTest points the cache at path and drops in-memory state.
// Tests must call this so they do not touch the real ~/.jarvis/cache.json.
func ResetForTest(path string) {
	mu.Lock()
	defer mu.Unlock()
	CACHE_PATH = path
	cache = nil
	dirty = false
}

func GetBoolCache(key string) (bool, bool) {
	value, found := GetCache(key)
	if !found {
		return false, false
	}

	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}

	return result, true
}

func SetBoolCache(key string, value bool) error {
	return SetCache(key, fmt.Sprintf("%t", value))
}

func GetInt64Cache(key string) (int64, bool) {
	value, found := GetCache(key)
	if !found {
		return 0, false
	}

	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false
	}

	return result, true
}

func SetInt64Cache(key string, value int64) error {
	return SetCache(key, fmt.Sprintf("%d", value))
}

func GetCache(key string) (string, bool) {
	mu.Lock()
	defer mu.Unlock()

	value, found := loadSimpleCache().Data[strings.ToLower(key)]
	if !found {
		return "", false
	}
	return value, true
}

func SetCache(key, value string) error {
	mu.Lock()
	defer mu.Unlock()
	c := loadSimpleCache()
	c.Data[strings.ToLower(key)] = value
	dirty = true
	return nil
}
