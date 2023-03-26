package common

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// RawTxToHash returns valid hex data of a transaction to
// transaction hash
func RawTxToHash(data string) string {
	return crypto.Keccak256Hash(hexutil.MustDecode(data)).Hex()
}

func BuildExactTx(nonce uint64, to string, ethAmount *big.Int, gasLimit uint64, priceGwei float64, data []byte) (tx *types.Transaction) {
	toAddress := common.HexToAddress(to)
	gasPrice := GweiToWei(priceGwei)
	return types.NewTransaction(nonce, toAddress, ethAmount, gasLimit, gasPrice, data)
}

func BuildTx(nonce uint64, to string, ethAmount float64, gasLimit uint64, priceGwei float64, data []byte) (tx *types.Transaction) {
	amount := FloatToBigInt(ethAmount, 18)
	return BuildExactTx(nonce, to, amount, gasLimit, priceGwei, data)
}

func BuildSendETHTx(nonce uint64, to string, ethAmount float64, priceGwei float64) (tx *types.Transaction) {
	return BuildTx(nonce, to, ethAmount, 30000, priceGwei, []byte{})
}

func BuildExactSendETHTx(nonce uint64, to string, ethAmount *big.Int, gasLimit uint64, priceGwei float64) (tx *types.Transaction) {
	return BuildExactTx(nonce, to, ethAmount, gasLimit, priceGwei, []byte{})
}

func BuildContractCreationTx(nonce uint64, ethAmount *big.Int, gasLimit uint64, priceGwei float64, data []byte) (tx *types.Transaction) {
	gasPrice := GweiToWei(priceGwei)
	return types.NewContractCreation(nonce, ethAmount, gasLimit, gasPrice, data)
}
