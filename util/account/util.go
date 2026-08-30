package account

import (
	"crypto/ecdsa"
	"io/ioutil"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
)

func AddressFromPrivateKey(key *ecdsa.PrivateKey) string {
	return crypto.PubkeyToAddress(key.PublicKey).Hex()
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
