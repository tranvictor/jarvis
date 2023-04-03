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
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/account"
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

func UnlockKeystoreAccountWithPassword(ad AccDesc, network networks.Network, pwd string) (*account.Account, error) {
	reader, err := util.EthReader(network)
	if err != nil {
		return nil, err
	}
	broadcaster, err := util.EthBroadcaster(network)
	if err != nil {
		return nil, err
	}

	fromAcc, err := account.NewKeystoreAccountGeneric(ad.Keypath, pwd, reader, broadcaster, network.GetChainID())
	if err != nil {
		fmt.Printf("Unlocking keystore '%s' failed: %s. Abort!\n", ad.Keypath, err)
		return nil, err
	}
	return fromAcc, nil
}

func UnlockAccount(ad AccDesc, network networks.Network) (*account.Account, error) {
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

		fromAcc, err = account.NewKeystoreAccountGeneric(ad.Keypath, pwd, reader, broadcaster, network.GetChainID())
		if err != nil {
			fmt.Printf("Unlocking keystore '%s' failed: %s. Abort!\n", ad.Keypath, err)
			return nil, err
		}
	case "trezor":
		fromAcc, err = account.NewTrezorAccountGeneric(ad.Derpath, ad.Address, reader, broadcaster, network.GetChainID())
		if err != nil {
			fmt.Printf("Creating trezor instance failed: %s\n", err)
			return nil, err
		}
	case "ledger", "ledger-live":
		fromAcc, err = account.NewLedgerAccountGeneric(ad.Derpath, ad.Address, reader, broadcaster, network.GetChainID())
		if err != nil {
			fmt.Printf("Creating ledger instance failed: %s\n", err)
			return nil, err
		}
	default:
		return nil, fmt.Errorf("not supported %s device", ad.Kind)
	}

	return fromAcc, nil
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
