package util

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
)

type stubReader struct {
	gasPrice float64
	nonce    uint64
	tip      float64
	dynamic  bool
	errPrice error
	errNonce error
	errTip   error
	errDyn   error
}

func (s stubReader) TxInfoFromHash(string) (jarviscommon.TxInfo, error) {
	return jarviscommon.TxInfo{}, errors.New("unused")
}
func (s stubReader) RecommendedGasPrice() (float64, error) { return s.gasPrice, s.errPrice }
func (s stubReader) GetMinedNonce(string) (uint64, error)  { return s.nonce, s.errNonce }
func (s stubReader) GetSuggestedGasTipCap() (float64, error) {
	return s.tip, s.errTip
}
func (s stubReader) CheckDynamicFeeTxAvailable() (bool, error) {
	return s.dynamic, s.errDyn
}
func (s stubReader) EstimateExactGas(string, string, float64, *big.Int, []byte) (uint64, error) {
	return 0, nil
}
func (s stubReader) EstimateGas(string, string, float64, float64, []byte) (uint64, error) {
	return 0, nil
}
func (s stubReader) GetBalance(string) (*big.Int, error)           { return big.NewInt(0), nil }
func (s stubReader) ERC20Balance(string, string) (*big.Int, error) { return big.NewInt(0), nil }
func (s stubReader) ERC20Decimal(string) (uint64, error)           { return 18, nil }
func (s stubReader) ReadContractToBytes(int64, string, string, *abi.ABI, string, ...interface{}) ([]byte, error) {
	return nil, nil
}

type nopBroadcaster struct{}

func (nopBroadcaster) BroadcastTx(*types.Transaction) (string, bool, error) {
	return "", false, nil
}

func resetSigningConfig(t *testing.T) {
	t.Helper()
	config.GasPrice = 0
	config.Nonce = 0
	config.TipGas = 0
	config.ForceLegacy = false
	t.Cleanup(func() {
		config.GasPrice = 0
		config.Nonce = 0
		config.TipGas = 0
		config.ForceLegacy = false
	})
}

func TestFillSigningTxParamsFetchesDefaults(t *testing.T) {
	resetSigningConfig(t)
	tc := TxContext{
		From:        "0xabc",
		Reader:      stubReader{gasPrice: 12, nonce: 7, tip: 1.5, dynamic: true},
		Broadcaster: nopBroadcaster{},
	}
	if err := FillSigningTxParams(ui.NewRecordingUI(), &tc, networks.EthereumMainnet); err != nil {
		t.Fatal(err)
	}
	if tc.GasPrice != 12 || tc.Nonce != 7 || tc.TipGas != 1.5 || tc.TxType != types.DynamicFeeTxType {
		t.Fatalf("got price=%v nonce=%d tip=%v type=%d", tc.GasPrice, tc.Nonce, tc.TipGas, tc.TxType)
	}
}

func TestFillSigningTxParamsHonorsFlags(t *testing.T) {
	resetSigningConfig(t)
	config.GasPrice = 30
	config.Nonce = 4
	config.TipGas = 2
	tc := TxContext{
		From:        "0xabc",
		Reader:      stubReader{dynamic: true},
		Broadcaster: nopBroadcaster{},
	}
	if err := FillSigningTxParams(ui.NewRecordingUI(), &tc, networks.EthereumMainnet); err != nil {
		t.Fatal(err)
	}
	if tc.GasPrice != 30 || tc.Nonce != 4 || tc.TipGas != 2 {
		t.Fatalf("got price=%v nonce=%d tip=%v", tc.GasPrice, tc.Nonce, tc.TipGas)
	}
}

func TestFillSigningTxParamsLegacyRejectsTip(t *testing.T) {
	resetSigningConfig(t)
	config.ForceLegacy = true
	config.TipGas = 1
	tc := TxContext{
		From:        "0xabc",
		Reader:      stubReader{},
		Broadcaster: nopBroadcaster{},
	}
	err := FillSigningTxParams(ui.NewRecordingUI(), &tc, networks.EthereumMainnet)
	if err == nil {
		t.Fatal("expected legacy+tip error")
	}
}
