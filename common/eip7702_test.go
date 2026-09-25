package common

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/holiman/uint256"
)

func TestBuildSetCodeTx(t *testing.T) {
	auth := types.SetCodeAuthorization{
		ChainID: *uint256.NewInt(1),
		Address: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:   4,
		V:       0,
		R:       *uint256.NewInt(1),
		S:       *uint256.NewInt(2),
	}
	tx := BuildSetCodeTx(9, "0x2222222222222222222222222222222222222222", big.NewInt(0), 100000, 20, 1, nil, 1, []types.SetCodeAuthorization{auth})
	got := TxAuthorizationsFrom(tx, nil)
	if len(got) != 1 || got[0].Nonce != "4" || got[0].Revoke {
		t.Fatalf("TxAuthorizationsFrom %+v", got)
	}

	revoke := auth
	revoke.Address = common.Address{}
	tx = BuildSetCodeTx(9, "0x2222222222222222222222222222222222222222", nil, 21000, 1, 1, nil, 1, []types.SetCodeAuthorization{revoke})
	got = TxAuthorizationsFrom(tx, nil)
	if len(got) != 1 || !got[0].Revoke {
		t.Fatalf("revoke %+v", got)
	}
}
