package reader

import (
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/util/explorers"
)

type delayNode struct {
	name       string
	txDelay    time.Duration
	recDelay   time.Duration
	tx         *jarviscommon.Transaction
	pending    bool
	txErr      error
	receipt    *types.Receipt
	receiptErr error
	txCalls    atomic.Int32
	recCalls   atomic.Int32
}

func (n *delayNode) NodeName() string { return n.name }
func (n *delayNode) NodeURL() string  { return "test://" + n.name }

func (n *delayNode) TransactionByHash(string) (*jarviscommon.Transaction, bool, error) {
	n.txCalls.Add(1)
	time.Sleep(n.txDelay)
	return n.tx, n.pending, n.txErr
}

func (n *delayNode) TransactionReceipt(string) (*types.Receipt, error) {
	n.recCalls.Add(1)
	time.Sleep(n.recDelay)
	return n.receipt, n.receiptErr
}

func (n *delayNode) EstimateGas(string, string, float64, *big.Int, []byte, *big.Int) (uint64, error) {
	return 0, nil
}
func (n *delayNode) GetCode(string) ([]byte, error)         { return nil, nil }
func (n *delayNode) GetBalance(string) (*big.Int, error)    { return big.NewInt(0), nil }
func (n *delayNode) GetMinedNonce(string) (uint64, error)   { return 0, nil }
func (n *delayNode) GetPendingNonce(string) (uint64, error) { return 0, nil }
func (n *delayNode) SuggestedGasPrice() (*big.Int, error)   { return big.NewInt(0), nil }
func (n *delayNode) SuggestedGasTipCap() (*big.Int, error)  { return big.NewInt(0), nil }
func (n *delayNode) ReadContractToBytes(int64, string, string, *abi.ABI, string, ...interface{}) ([]byte, error) {
	return nil, nil
}
func (n *delayNode) EthCall(string, string, *big.Int, []byte, *map[ethcommon.Address]gethclient.OverrideAccount) ([]byte, error) {
	return nil, nil
}
func (n *delayNode) CallAtBlock(string, string, *big.Int, uint64, []byte, *big.Int) ([]byte, error) {
	return nil, nil
}
func (n *delayNode) StorageAt(int64, string, string) ([]byte, error) { return nil, nil }
func (n *delayNode) HeaderByNumber(int64) (*types.Header, error)     { return nil, nil }
func (n *delayNode) GetLogs(int, int, []string, string) ([]types.Log, error) {
	return nil, nil
}
func (n *delayNode) CurrentBlock() (uint64, error) { return 0, nil }

type stubExplorer struct{}

func (stubExplorer) GetABIString(string) (string, error) { return "", nil }
func (stubExplorer) GetContractInfo(string) (explorers.ContractInfo, error) {
	return explorers.ContractInfo{}, nil
}

func TestTxInfoFromHashFetchesReceiptInParallel(t *testing.T) {
	to := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	inner := types.NewTx(&types.LegacyTx{To: &to, Gas: 21000})
	node := &delayNode{
		name:     "mock",
		txDelay:  80 * time.Millisecond,
		recDelay: 80 * time.Millisecond,
		tx:       &jarviscommon.Transaction{Transaction: inner},
		receipt:  &types.Receipt{Status: 1, GasUsed: 21000},
	}
	er := &EthReader{
		nodes: map[string]EthereumNode{"mock": node},
		be:    stubExplorer{},
	}

	start := time.Now()
	info, err := er.TxInfoFromHash("0xabc")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "done" || info.Receipt == nil || info.Tx == nil {
		t.Fatalf("info: %+v", info)
	}
	if node.txCalls.Load() != 1 || node.recCalls.Load() != 1 {
		t.Fatalf("calls tx=%d receipt=%d", node.txCalls.Load(), node.recCalls.Load())
	}
	if elapsed > 140*time.Millisecond {
		t.Fatalf("expected overlapping fetches, took %s", elapsed)
	}
}

func TestTxInfoFromHashPendingDoesNotWaitOnReceiptError(t *testing.T) {
	to := ethcommon.HexToAddress("0x2222222222222222222222222222222222222222")
	inner := types.NewTx(&types.LegacyTx{To: &to, Gas: 21000})
	node := &delayNode{
		name:     "mock",
		tx:       &jarviscommon.Transaction{Transaction: inner},
		pending:  true,
		receipt:  nil,
		recDelay: 10 * time.Millisecond,
	}
	er := &EthReader{
		nodes: map[string]EthereumNode{"mock": node},
		be:    stubExplorer{},
	}
	info, err := er.TxInfoFromHash("0xabc")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "pending" || info.Receipt != nil {
		t.Fatalf("pending info: %+v", info)
	}
}
