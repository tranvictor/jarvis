package common

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tranvictor/jarvis/networks"
)

// RawTxToHash returns valid hex data of a transaction to
// transaction hash
func RawTxToHash(data string) string {
	return crypto.Keccak256Hash(hexutil.MustDecode(data)).Hex()
}

func BuildExactTx(
	nonce uint64,
	to string,
	ethAmount *big.Int,
	gasLimit uint64,
	priceGwei float64,
	tipCapGwei float64,
	data []byte,
) (tx *types.Transaction) {
	toAddress := common.HexToAddress(to)
	gasPrice := GweiToWei(priceGwei)
	tipInt := GweiToWei(tipCapGwei)
	chainIDInt := big.NewInt(networks.CurrentNetwork().GetChainID())
	if tipInt.Cmp(common.Big0) > 0 { // dynamyc fee tx
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainIDInt,
			Nonce:     nonce,
			GasTipCap: tipInt,
			GasFeeCap: gasPrice,
			Gas:       gasLimit,
			To:        &toAddress,
			Value:     ethAmount,
			Data:      data,
		})
	} else {
		return types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gasLimit,
			To:       &toAddress,
			Value:    ethAmount,
			Data:     data,
		})
	}
}

func BuildTx(
	nonce uint64,
	to string,
	ethAmount float64,
	gasLimit uint64,
	priceGwei float64,
	tipCapGwei float64,
	data []byte,
) (tx *types.Transaction) {
	amount := FloatToBigInt(ethAmount, 18)
	return BuildExactTx(nonce, to, amount, gasLimit, priceGwei, tipCapGwei, data)
}

func BuildExactSendETHTx(
	nonce uint64,
	to string,
	ethAmount *big.Int,
	gasLimit uint64,
	priceGwei float64,
	tipCapGwei float64,
	chainID int64,
) (tx *types.Transaction) {
	return BuildExactTx(nonce, to, ethAmount, gasLimit, priceGwei, tipCapGwei, []byte{})
}

func BuildContractCreationTx(
	nonce uint64,
	ethAmount *big.Int,
	gasLimit uint64,
	priceGwei float64,
	tipCapGwei float64,
	data []byte,
) (tx *types.Transaction) {
	gasPrice := GweiToWei(priceGwei)
	tipInt := GweiToWei(tipCapGwei)
	chainIDInt := big.NewInt(networks.CurrentNetwork().GetChainID())
	if tipInt.Cmp(common.Big0) > 0 { // dynamyc fee tx
		return types.NewTx(&types.DynamicFeeTx{
			ChainID:   chainIDInt,
			Nonce:     nonce,
			GasTipCap: tipInt,
			GasFeeCap: gasPrice,
			Gas:       gasLimit,
			Value:     ethAmount,
			Data:      data,
		})
	} else {
		return types.NewTx(&types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gasLimit,
			Value:    ethAmount,
			Data:     data,
		})
	}
}
