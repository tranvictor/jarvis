package common

import (
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

type InternalTx struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Value string `json:"value"`
}

type TxInfo struct {
	Status      string
	Tx          *Transaction
	InternalTxs []InternalTx
	Receipt     *types.Receipt
	// BlockHeader *types.Header
}

func (self *TxInfo) GasCost() *big.Int {
	return big.NewInt(0).Mul(
		big.NewInt(int64(self.Receipt.GasUsed)),
		self.Tx.GasPrice(),
	)
}

type Transaction struct {
	*types.Transaction
	Extra TxExtraInfo `json:"extra"`
}

type TxExtraInfo struct {
	BlockNumber *string         `json:"blockNumber,omitempty"`
	BlockHash   *common.Hash    `json:"blockHash,omitempty"`
	From        *common.Address `json:"from,omitempty"`
}

func (tx *Transaction) UnmarshalJSON(msg []byte) error {
	if err := json.Unmarshal(msg, &tx.Transaction); err != nil {
		return err
	}
	return json.Unmarshal(msg, &tx.Extra)
}

type txmarshaling struct {
	AccountNonce hexutil.Uint64  `json:"nonce"`
	Price        *hexutil.Big    `json:"gasPrice"`
	GasLimit     hexutil.Uint64  `json:"gas"`
	Recipient    *common.Address `json:"to"`
	Amount       *hexutil.Big    `json:"value"`
	Payload      hexutil.Bytes   `json:"input"`
	BlockNumber  *string         `json:"blockNumber,omitempty"`
	BlockHash    *common.Hash    `json:"blockHash,omitempty"`
	From         *common.Address `json:"from,omitempty"`

	// Signature values
	V *hexutil.Big `json:"v"`
	R *hexutil.Big `json:"r"`
	S *hexutil.Big `json:"s"`

	// This is only used when marshaling to JSON.
	Hash *common.Hash `json:"hash"`
}

func (tx *Transaction) MarshalJSON() ([]byte, error) {
	v, r, s := tx.Transaction.RawSignatureValues()
	h := tx.Transaction.Hash()
	txmar := txmarshaling{
		AccountNonce: hexutil.Uint64(tx.Transaction.Nonce()),
		Price:        (*hexutil.Big)(tx.Transaction.GasPrice()),
		GasLimit:     hexutil.Uint64(tx.Transaction.Gas()),
		Recipient:    tx.Transaction.To(),
		Amount:       (*hexutil.Big)(tx.Transaction.Value()),
		Payload:      hexutil.Bytes(tx.Transaction.Data()),
		BlockNumber:  tx.Extra.BlockNumber,
		BlockHash:    tx.Extra.BlockHash,
		From:         tx.Extra.From,
		V:            (*hexutil.Big)(v),
		R:            (*hexutil.Big)(r),
		S:            (*hexutil.Big)(s),
		Hash:         &h,
	}
	return json.Marshal(txmar)
}
