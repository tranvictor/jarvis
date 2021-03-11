package accounts

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"

	gethkeystore "github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/sahilm/fuzzy"
	"github.com/tranvictor/ethutils/account"
	"github.com/tranvictor/jarvis/util"
	"golang.org/x/crypto/ssh/terminal"
)

type AccDesc struct {
	Address string
	Kind    string
	Keypath string
	Derpath string
	Desc    string
}

func getHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	return usr.HomeDir
}

func getPassword(prompt string) string {
	fmt.Print(prompt)
	bytePassword, _ := terminal.ReadPassword(int(syscall.Stdin))
	return string(bytePassword)
}

type keystore struct {
	Address string `json:"address"`
}

func StorePrivateKeyWithKeystore(privateKey string, passphrase string) (string, error) {
	priv, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return "", err
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	key := &gethkeystore.Key{
		Id:         id,
		Address:    crypto.PubkeyToAddress(priv.PublicKey),
		PrivateKey: priv,
	}

	keystoreJson, err := gethkeystore.EncryptKey(
		key,
		passphrase,
		262144, // n
		1,      // p
	)
	if err != nil {
		return "", nil
	}

	dir := filepath.Join(getHomeDir(), ".jarvis", "keystores")
	os.MkdirAll(dir, os.ModePerm)
	path := filepath.Join(dir, fmt.Sprintf("%s.json", key.Address))
	return path, ioutil.WriteFile(path, keystoreJson, 0644)
}

func VerifyKeystore(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	k := &keystore{}
	err = json.Unmarshal(content, k)
	if err != nil {
		return "", err
	}
	return "0x" + k.Address, nil
}

func StoreAccountRecord(accDesc AccDesc) error {
	dir := filepath.Join(getHomeDir(), ".jarvis")
	os.MkdirAll(dir, os.ModePerm)
	path := filepath.Join(dir, fmt.Sprintf("%s.json", accDesc.Address))
	content, _ := json.Marshal(accDesc)
	return ioutil.WriteFile(path, content, 0644)
}

func UnlockAccount(ad AccDesc, network string) (*account.Account, error) {
	var fromAcc *account.Account
	var err error

	reader, err := util.EthReader(network)
	if err != nil {
		return nil, err
	}
	broadcaster, err := util.EthBroadcaster(network)
	if err != nil {
		return nil, err
	}

	switch ad.Kind {
	case "keystore":
		fmt.Printf("Using keystore: %s\n", ad.Keypath)
		pwd := getPassword("Enter passphrase: ")
		fmt.Printf("\n")
		if network == "mainnet" {
			fromAcc, err = account.NewAccountFromKeystore(ad.Keypath, pwd)
		} else if network == "ropsten" {
			fromAcc, err = account.NewRopstenAccountFromKeystore(ad.Keypath, pwd)
		} else if network == "kovan" {
			fromAcc, err = account.NewKovanAccountFromKeystore(ad.Keypath, pwd)
		} else if network == "rinkeby" {
			fromAcc, err = account.NewRinkebyAccountFromKeystore(ad.Keypath, pwd)
		} else if network == "tomo" {
			fromAcc, err = account.NewTomoAccountFromKeystore(ad.Keypath, pwd)
		} else {
			return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten")
		}
		if err != nil {
			fmt.Printf("Unlocking keystore '%s' failed: %s. Abort!\n", ad.Keypath, err)
			return nil, err
		}
		fromAcc.SetReader(reader)
		fromAcc.SetBroadcaster(broadcaster)
		return fromAcc, nil
	case "trezor":
		if network == "mainnet" {
			fromAcc, err = account.NewTrezorAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "ropsten" {
			fromAcc, err = account.NewRopstenTrezorAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "kovan" {
			fromAcc, err = account.NewKovanTrezorAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "rinkeby" {
			fromAcc, err = account.NewRinkebyTrezorAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "tomo" {
			fromAcc, err = account.NewTomoTrezorAccount(
				ad.Derpath, ad.Address,
			)
		} else {
			return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten")
		}
		if err != nil {
			fmt.Printf("Creating trezor instance failed: %s\n", err)
			return nil, err
		}
		fromAcc.SetReader(reader)
		fromAcc.SetBroadcaster(broadcaster)
		return fromAcc, nil
	case "ledger":
		if network == "mainnet" {
			fromAcc, err = account.NewLedgerAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "ropsten" {
			fromAcc, err = account.NewRopstenLedgerAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "kovan" {
			fromAcc, err = account.NewKovanLedgerAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "rinkeby" {
			fromAcc, err = account.NewRinkebyLedgerAccount(
				ad.Derpath, ad.Address,
			)
		} else if network == "tomo" {
			fromAcc, err = account.NewTomoLedgerAccount(
				ad.Derpath, ad.Address,
			)
		} else {
			return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten")
		}
		if err != nil {
			fmt.Printf("Creating ledger instance failed: %s\n", err)
			return nil, err
		}
		fromAcc.SetReader(reader)
		fromAcc.SetBroadcaster(broadcaster)
		return fromAcc, nil
	}
	return nil, nil
}

func GetAccount(input string) (AccDesc, error) {
	source := NewFuzzySource()
	matches := fuzzy.FindFrom(strings.Replace(input, " ", "_", -1), source)
	if len(matches) == 0 {
		return AccDesc{}, fmt.Errorf("No account is found with '%s'", input)
	}
	match := matches[0]
	return source[match.Index], nil
}

// GetAccounts returns a map address -> account description
// Each description is stored in a json file whose name is
// the address and content is the description.
// All files are kept in ~/.jarvis/
func GetAccounts() map[string]AccDesc {
	paths, err := filepath.Glob(filepath.Join(getHomeDir(), ".jarvis", "*.json"))
	if err != nil {
		fmt.Printf("Getting accounts failed: %s.\n", err)
		return map[string]AccDesc{}
	}
	result := map[string]AccDesc{}
	for _, p := range paths {
		desc := AccDesc{}
		content, err := ioutil.ReadFile(p)
		if err == nil {
			err = json.Unmarshal(content, &desc)
			if err != nil {
				fmt.Printf("Reading account %s description failed: %s. Ignore and continue.\n", p, err)
			} else {
				addr, err := util.PathToAddress(p)
				if err == nil {
					result[addr] = desc
				}
			}
		} else {
			fmt.Printf("Reading account description failed: %s. Ignore and continue.\n", err)
		}
	}
	return result
}
