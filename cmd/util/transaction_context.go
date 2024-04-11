package util

// import (
// 	"fmt"
// 	"math/big"
//
// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/ethereum/go-ethereum/core/types"
// 	"github.com/tranvictor/jarvis/networks"
// )
//
// type TransactionContext struct {
// 	cm              *ContextManager
// 	network         networks.Network
// 	to              *common.Address
// 	from            *common.Address
// 	createdContract *common.Address
// 	value           *big.Int
// 	data            []byte
// 	gasLimit        *big.Int
// 	gasPrice        *big.Int
// 	nonce           *uint64
//
// 	tx *types.Transaction
// }
//
// func (tc *TransactionContext) SetNetwork(name string) (err error) {
// 	tc.network, err = networks.GetNetwork(name)
// 	return fmt.Errorf("couldn't set network in transaction context: %s", err)
// }
//
// func (tc *TransactionContext) SetValue(str string) (err error) {
// 	tc.Value, err = FloatStringToBig(str, tc.network.GetNativeTokenDecimal())
// 	if err != nil {
// 		return fmt.Errorf("couldn't set value in transaction context: %s", err)
// 	}
// }
//
// func (tc *TransactionContext) SetToAndFrom(toStr string) error {
// }
//
// func (tc *TransactionContext) SetData(data []byte) {
// 	tc.data = data
// }
//
// func (tc *TransactionContext) SetGasLimit(limit int64) {
// 	tc.gasLimit = big.NewInt(limit)
// }
//
// func (tc *TransactionContext) SetGasPrice(price *big.Int) {
// 	tc.gasPrice = price
// }
//
// func (tc *TransactionContext) SetNonce(nonce uint64) {
// 	tc.nonce = &nonce
// }
//
// func (tc *TransactionContext) BuildTx() (*types.Transaction, error) {
// 	if tc.nonce == nil {
// 		nonceBig, err := tc.cm.Nonce(tc.from, tc.network)
// 		if err != nil {
// 			return nil, fmt.Errorf("couldn't get nonce in transaction context: %s", err)
// 		}
// 		nonce := nonceBig.Uint64()
// 		tc.nonce = &nonce
// 	}
//
// 	tc.tx = BuildExactTx(
// 		tc.nonce,
// 		tc.to.Hex(),
// 		tc.value,
// 		tc.gasLimit,
// 		tc.gasPrice,
// 		tc.data,
// 	)
// }
