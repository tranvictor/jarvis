package account

import (
	"crypto/ecdsa"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
)

func SeedToPrivateKey(seed string) (string, *ecdsa.PrivateKey) {
	result, _ := crypto.ToECDSA(crypto.Keccak256([]byte(seed)))
	pubhex := crypto.PubkeyToAddress(result.PublicKey).Hex()
	return pubhex, result
}

func RandomPrivateKey(seed string) (string, *ecdsa.PrivateKey) {
	rand.Seed(time.Now().UnixNano())
	ran := []byte("12345678123456781234567812345678")
	rand.Read(ran)
	return SeedToPrivateKey(seed + string(ran))
}

func AddressFromPrivateKey(key *ecdsa.PrivateKey) string {
	return crypto.PubkeyToAddress(key.PublicKey).Hex()
}

func AddressFromHex(hex string) (string, error) {
	key, err := crypto.HexToECDSA(hex[2:])
	if err != nil {
		return "", err
	}
	return AddressFromPrivateKey(key), nil
}

func PrivateKeyFromKeystore(file string, password string) (string, *ecdsa.PrivateKey, error) {
	json, err := ioutil.ReadFile(file)
	if err != nil {
		return "", nil, err
	}
	key, err := keystore.DecryptKey(json, password)
	if err != nil {
		return "", nil, err
	}
	pubhex := AddressFromPrivateKey(key.PrivateKey)
	return pubhex, key.PrivateKey, nil
}

// works with both 0x prefix form and naked form
func PrivateKeyFromHex(hex string) (string, *ecdsa.PrivateKey, error) {
	if hex[0:2] == "0x" {
		hex = hex[2:]
	}
	privkey, err := crypto.HexToECDSA(hex)
	if err != nil {
		return "", nil, err
	} else {
		pubhex := AddressFromPrivateKey(privkey)
		return pubhex, privkey, nil
	}
}

func PrivateKeyFromFile(file string) (string, *ecdsa.PrivateKey, error) {
	privkey, err := crypto.LoadECDSA(file)
	if err != nil {
		return "", nil, err
	} else {
		pubhex := AddressFromPrivateKey(privkey)
		return pubhex, privkey, nil
	}
}

// Generate a private key from a seed using keccak256 and
// store the private key in hex format to a file with
// generated address as its name. The file is saved to
// current working directory if `dir` is "", otherwise it
// will be saved to `dir`.
func SeedToPrivateKeyFile(seed string, dir string) (filepath string, privkey *ecdsa.PrivateKey, err error) {
	pubhex, privkey := SeedToPrivateKey(seed)
	filepath = path.Join(path.Clean(dir), pubhex)
	os.MkdirAll(path.Clean(dir), os.ModePerm)
	return filepath, privkey, crypto.SaveECDSA(
		filepath, privkey)
}

func RandomPrivateKeyFile(seed string, dir string) (string, *ecdsa.PrivateKey, error) {
	rand.Seed(time.Now().UnixNano())
	ran := []byte("12345678123456781234567812345678")
	rand.Read(ran)
	return SeedToPrivateKeyFile(seed+string(ran), dir)
}
