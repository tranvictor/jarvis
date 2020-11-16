package bleve

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/analysis/lang/en"
	"github.com/blevesearch/bleve/mapping"
	"github.com/tranvictor/jarvis/db"
)

var (
	BLEVE_PATH      string = filepath.Join(getHomeDir(), ".jarvis", "db.bleve")
	BLEVE_DATA_PATH string = filepath.Join(getHomeDir(), ".jarvis", "bleve.data")
	bleveDB         *BleveDB
	once            sync.Once
)

func getHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr.HomeDir
}

func getDataFromDefaultFile() (result map[string]string, hash string) {
	usr, _ := user.Current()
	dir := usr.HomeDir
	file := path.Join(dir, "addresses.json")
	var timestamp int64
	fi, err := os.Lstat(file)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}, fmt.Sprintf("%d", timestamp)
	}
	// if the file is a symlink
	if fi.Mode()&os.ModeSymlink != 0 {
		file, err = os.Readlink(file)
		if err != nil {
			fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
			return map[string]string{}, fmt.Sprintf("%d", timestamp)
		}
	}
	content, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}, fmt.Sprintf("%d", timestamp)
	}

	info, err := os.Stat(file)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}, fmt.Sprintf("%d", timestamp)
	}
	timestamp += info.ModTime().UnixNano()

	err = json.Unmarshal(content, &result)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}, fmt.Sprintf("%d", timestamp)
	}

	content, err = ioutil.ReadFile(path.Join(dir, "secrets.json"))
	if err == nil {
		secret := map[string]string{}
		err = json.Unmarshal(content, &secret)
		if err == nil {
			for addr, name := range secret {
				result[addr] = name
			}
		}
	}
	info, err = os.Stat(path.Join(dir, "secrets.json"))
	if err == nil {
		timestamp += info.ModTime().UnixNano()
	}

	for addr, tokenName := range db.TOKENS {
		result[addr] = tokenName
	}
	return result, fmt.Sprintf("%d", timestamp)
}

type BleveDB struct {
	index bleve.Index
	Hash  string
}

func buildIndexMapping() mapping.IndexMapping {
	textFieldMapping := bleve.NewTextFieldMapping()
	textFieldMapping.Analyzer = en.AnalyzerName

	defaultMapping := bleve.NewDocumentMapping()
	defaultMapping.AddFieldMappingsAt("desc",
		textFieldMapping)

	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("_default", defaultMapping)

	indexMapping.TypeField = "type"
	indexMapping.DefaultAnalyzer = "en"

	return indexMapping
}

func loadIndex(db *BleveDB) error {
	index, err := bleve.Open(BLEVE_DATA_PATH)
	if err != nil && err != bleve.ErrorIndexPathDoesNotExist {
		return err
	}

	if err == nil {
		db.index = index
	}

	addrs, h := getDataFromDefaultFile()

	if err == bleve.ErrorIndexPathDoesNotExist {
		// here index file doesn't exist, create one
		indexMapping := buildIndexMapping()
		index, err = bleve.New(BLEVE_DATA_PATH, indexMapping)
		if err != nil {
			return err
		}
		db.index = index
		db.Hash = ""
	}

	if db.Hash != h {
		err = indexAddresses(bleveDB.index, addrs)
		if err != nil {
			return err
		}
		db.Hash = h
		return db.Persist()
	}
	return nil
}

func loadBleveDB() (*BleveDB, error) {
	result := &BleveDB{}
	content, err := ioutil.ReadFile(BLEVE_PATH)
	if err != nil {
		return result, nil
	}
	err = json.Unmarshal(content, result)
	if err != nil {
		return result, nil
	}

	return result, nil
}

func NewBleveDB() (*BleveDB, error) {
	var resError error
	once.Do(func() {
		bleveDB, resError = loadBleveDB()
		if resError != nil {
			return
		}
		resError = loadIndex(bleveDB)
	})
	return bleveDB, resError
}

func (bleveDB *BleveDB) Persist() error {
	jsonData, err := json.MarshalIndent(bleveDB, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(BLEVE_PATH, jsonData, 0644)
}

func (bleveDB *BleveDB) Search(input string) ([]AddressDesc, []int) {
	matchQuery := bleve.NewMatchPhraseQuery(input)
	fuzzyQuery := bleve.NewFuzzyQuery(input)
	fuzzyQuery.Fuzziness = 1
	query := bleve.NewDisjunctionQuery(matchQuery, fuzzyQuery)
	request := bleve.NewSearchRequest(query)
	searchResults, err := bleveDB.index.Search(request)
	if err != nil {
		fmt.Printf("Address db search failed: %s\n", err)
		return []AddressDesc{}, []int{}
	}

	results := []AddressDesc{}
	resultScores := []int{}
	for _, searchResult := range searchResults.Hits {
		doc, err := bleveDB.index.Document(searchResult.ID)
		if err != nil {
			fmt.Printf("getting address data for %s failed: %s. Ignored.", searchResult.ID, err)
			continue
		}
		resultScores = append(resultScores, int(searchResult.Score*1000000))
		results = append(results, AddressDesc{
			Address: string(doc.Fields[0].Value()),
			Desc:    string(doc.Fields[1].Value()),
		})
	}
	return results, resultScores
}

func indexAddresses(i bleve.Index, addrs map[string]string) error {
	startTime := time.Now().UnixNano()
	batch := i.NewBatch()
	batchCount := 0
	fmt.Printf("indexing %s addresses\n", len(addrs))
	for addr, desc := range addrs {
		batch.Index(addr, AddressDesc{
			Address: addr,
			Desc:    desc,
		})
		batchCount++

		if batchCount >= 1000 {
			err := i.Batch(batch)
			if err != nil {
				return err
			}
			batch = i.NewBatch()
			batchCount = 0
		}
	}
	// flush the last batch
	if batchCount > 0 {
		err := i.Batch(batch)
		if err != nil {
			return err
		}
	}
	endTime := time.Now().UnixNano()
	fmt.Printf("Total index time: %d ms\n", (endTime-startTime)/1000000)
	return nil
}
