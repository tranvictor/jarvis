package db

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	"github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

type DefaultAddressDatabase struct {
	Data map[common.Address]string
}

func (self *DefaultAddressDatabase) Register(addr string, name string) {
	// HexToAddress maps invalid or truncated hex (and any non-hex string)
	// onto address(0). A reversed or typo'd addresses.json entry would
	// otherwise attach someone's name to the zero address.
	if !jarviscommon.LooksLikeAddress(addr) {
		return
	}
	a := common.HexToAddress(addr)
	if a == (common.Address{}) {
		return
	}
	self.Data[a] = name
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
		// Having no personal address book is the normal state for a fresh
		// install; only a file that exists but can't be read is worth a note.
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		}
		return map[string]string{}
	}
	// if the file is a symlink
	if fi.Mode()&os.ModeSymlink != 0 {
		file, err = os.Readlink(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
			return map[string]string{}
		}
	}
	content, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
		return map[string]string{}
	}
	result := map[string]string{}
	err = json.Unmarshal(content, &result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading addresses from ~/addresses.json failed: %s. Ignored.\n", err)
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
