package db

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	"github.com/ethereum/go-ethereum/common"
)

type DefaultAddressDatabase struct {
	Data map[common.Address]string
}

func (self *DefaultAddressDatabase) Register(addr string, name string) {
	self.Data[common.HexToAddress(addr)] = name
}

func (self *DefaultAddressDatabase) GetName(addr string) string {
	name, found := self.Data[common.HexToAddress(addr)]
	if found {
		return name
	} else {
		return "unknown"
	}
}

type tokens []struct {
	Address string `json:"address"`
	Symbol  string `json:"symbol"`
}

func registerTokens(db *DefaultAddressDatabase) error {
	tokens := AllTokenAddresses()
	for addr, symbol := range tokens {
		db.Register(addr, symbol)
	}
	return nil
}

func getDataFromDefaultFile() map[string]string {
	usr, _ := user.Current()
	dir := usr.HomeDir
	file := path.Join(dir, "addresses.json")
	fi, err := os.Lstat(file)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}
	}
	// if the file is a symlink
	if fi.Mode()&os.ModeSymlink != 0 {
		file, err = os.Readlink(file)
		if err != nil {
			fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
			return map[string]string{}
		}
	}
	content, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}
	}
	result := map[string]string{}
	err = json.Unmarshal(content, &result)
	if err != nil {
		fmt.Printf("reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}
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
	return result
}

func NewDefaultAddressDatabase() *DefaultAddressDatabase {

	// get data from ~/addresses.json, expecting a map
	// from address (string) to name (string)
	data := getDataFromDefaultFile()

	db := &DefaultAddressDatabase{
		Data: map[common.Address]string{},
	}

	err := registerTokens(db)
	if err != nil {
		fmt.Printf("Loading token addresses from file failed: %s\n", err)
	}

	for addr, name := range data {
		db.Register(addr, name)
	}

	return db
}
