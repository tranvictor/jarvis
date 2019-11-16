package cache

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
)

var (
	CACHE_PATH string = filepath.Join(getHomeDir(), ".jarvis", "cache.json")
	cache      *simpleCache
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

func (self *simpleCache) Persist() error {
	jsonData, err := json.MarshalIndent(self, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(CACHE_PATH, jsonData, 0644)
}

func loadSimpleCache() *simpleCache {
	if cache != nil {
		return cache
	}
	cache = &simpleCache{
		Data: map[string]string{},
	}
	content, err := ioutil.ReadFile(CACHE_PATH)
	if err != nil {
		// WARNING: swallow error here
		return cache
	}
	err = json.Unmarshal(content, cache)
	if err != nil {
		// WARNING: swallow error here
		return cache
	}
	return cache
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
	return cache.Persist()
}
