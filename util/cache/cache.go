package cache

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os/user"
	"path/filepath"
	"strconv"
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
	return cache.Persist()
}
