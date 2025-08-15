package reader

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

const TIMEOUT time.Duration = 4 * time.Second

type OneNodeReader struct {
	nodeName   string
	nodeURL    string
	client     *rpc.Client
	ethClient  *ethclient.Client
	gethClient *gethclient.Client
	mu         sync.Mutex
}

func NewOneNodeReader(name, url string) *OneNodeReader {
	return &OneNodeReader{
		nodeName:   name,
		nodeURL:    url,
		client:     nil,
		ethClient:  nil,
		gethClient: nil,
		mu:         sync.Mutex{},
	}
}

func (onr *OneNodeReader) NodeName() string {
	return onr.nodeName
}

func (onr *OneNodeReader) NodeURL() string {
	return onr.nodeURL
}

func (onr *OneNodeReader) initConnection() error {
	onr.mu.Lock()
	defer onr.mu.Unlock()
	client, err := rpc.Dial(onr.NodeURL())
	if err != nil {
		return fmt.Errorf("couldn't connect to %s: %w", onr.nodeName, err)
	}
	onr.client = client
	onr.ethClient = ethclient.NewClient(onr.client)
	onr.gethClient = gethclient.New(onr.client)
	return nil
}

func (onr *OneNodeReader) Client() (*rpc.Client, error) {
	if onr.client != nil {
		return onr.client, nil
	}
	err := onr.initConnection()
	return onr.client, err
}

func (onr *OneNodeReader) EthClient() (*ethclient.Client, error) {
	if onr.ethClient != nil {
		return onr.ethClient, nil
	}
	err := onr.initConnection()
	return onr.ethClient, err
}

func (onr *OneNodeReader) GEthClient() (*gethclient.Client, error) {
	if onr.gethClient != nil {
		return onr.gethClient, nil
	}
	err := onr.initConnection()
	return onr.gethClient, err
}

func (onr *OneNodeReader) EstimateGas(from, to string, priceGwei float64, value *big.Int, data []byte) (uint64, error) {
	fromAddr := common.HexToAddress(from)
	var toAddrPtr *common.Address
	if to != "" {
		toAddr := common.HexToAddress(to)
		toAddrPtr = &toAddr
	}
	price := jarviscommon.FloatToBigInt(priceGwei, 9)
	ethcli, err := onr.EthClient()
	if err != nil {
		return 0, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.EstimateGas(timeout, ethereum.CallMsg{
		From:     fromAddr,
		To:       toAddrPtr,
		Gas:      0,
		GasPrice: price,
		Value:    value,
		Data:     data,
	})
}

func (onr *OneNodeReader) GetCode(address string) (code []byte, err error) {
	addr := common.HexToAddress(address)
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.CodeAt(timeout, addr, nil)
}

func (onr *OneNodeReader) GetGasPriceSuggestion() (*big.Int, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.SuggestGasPrice(timeout)
}

func (onr *OneNodeReader) GetBalance(address string) (balance *big.Int, err error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}
	acc := common.HexToAddress(address)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.BalanceAt(timeout, acc, nil)
}

func (onr *OneNodeReader) GetMinedNonce(address string) (nonce uint64, err error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return 0, err
	}
	acc := common.HexToAddress(address)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.NonceAt(timeout, acc, nil)
}

func (onr *OneNodeReader) GetPendingNonce(address string) (nonce uint64, err error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return 0, err
	}
	acc := common.HexToAddress(address)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.PendingNonceAt(timeout, acc)
}

func (onr *OneNodeReader) TransactionReceipt(txHash string) (receipt *types.Receipt, err error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}
	hash := common.HexToHash(txHash)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.TransactionReceipt(timeout, hash)
}

func (onr *OneNodeReader) transactionByHashOnNode(ctx context.Context, hash common.Hash) (tx *jarviscommon.Transaction, isPending bool, err error) {
	var json *jarviscommon.Transaction
	cli, err := onr.Client()
	if err != nil {
		return nil, false, err
	}
	err = cli.CallContext(ctx, &json, "eth_getTransactionByHash", hash)
	if err != nil {
		return nil, false, err
	} else if json == nil {
		return nil, false, ethereum.NotFound
	} else if _, r, _ := json.RawSignatureValues(); r == nil {
		return nil, false, fmt.Errorf("server returned transaction without signature")
	}
	return json, json.Extra.BlockNumber == nil, nil
}

func (onr *OneNodeReader) TransactionByHash(txHash string) (tx *jarviscommon.Transaction, isPending bool, err error) {
	hash := common.HexToHash(txHash)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return onr.transactionByHashOnNode(timeout, hash)
}

// func (onr *OneNodeReader) Call(result interface{}, method string, args ...interface{}) error {
// 	cli, err := onr.Client()
// 	if err != nil {
// 		return err
// 	}
// 	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
// 	defer cancel()
// 	return cli.CallContext(timeout, result, method, args)
// }

func (onr *OneNodeReader) HeaderByNumber(number int64) (*types.Header, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}
	var numberBig *big.Int
	if number > -1 {
		numberBig = big.NewInt(number)
	}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return ethcli.HeaderByNumber(timeout, numberBig)
}

func (onr *OneNodeReader) SuggestedGasPrice() (*big.Int, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}

	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	return ethcli.SuggestGasPrice(timeout)
}

func (onr *OneNodeReader) SuggestedGasTipCap() (*big.Int, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}

	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	return ethcli.SuggestGasTipCap(timeout)
}

func (onr *OneNodeReader) GetLogs(fromBlock, toBlock int, addresses []string, topic string) ([]types.Log, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}

	q := &ethereum.FilterQuery{}
	q.BlockHash = nil
	q.FromBlock = big.NewInt(int64(fromBlock))
	if toBlock < 0 {
		q.ToBlock = nil
	} else {
		q.ToBlock = big.NewInt(int64(toBlock))
	}
	q.Addresses = jarviscommon.HexToAddresses(addresses)
	q.Topics = [][]common.Hash{
		{jarviscommon.HexToHash(topic)},
	}

	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.FilterLogs(timeout, *q)
}

func (onr *OneNodeReader) ReadContractToBytes(atBlock int64, from string, caddr string, abi *abi.ABI, method string, args ...interface{}) ([]byte, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return nil, err
	}

	contract := jarviscommon.HexToAddress(caddr)
	data, err := abi.Pack(method, args...)
	if err != nil {
		return nil, err
	}

	var blockBig *big.Int
	if atBlock > 0 {
		blockBig = big.NewInt(atBlock)
	}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	return ethcli.CallContract(timeout, ethereum.CallMsg{
		From:     jarviscommon.HexToAddress(from),
		To:       &contract,
		Gas:      0,
		GasPrice: nil,
		Value:    nil,
		Data:     data,
	}, blockBig)
}

func (onr *OneNodeReader) EthCall(from string, to string, data []byte, overrides *map[common.Address]gethclient.OverrideAccount) ([]byte, error) {
	gethcli, err := onr.GEthClient()
	if err != nil {
		return nil, err
	}

	contract := jarviscommon.HexToAddress(to)

	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return gethcli.CallContract(timeout, ethereum.CallMsg{
		From:     jarviscommon.HexToAddress(from),
		To:       &contract,
		Gas:      0,
		GasPrice: nil,
		Value:    nil,
		Data:     data,
	}, nil, overrides)
}

func (onr *OneNodeReader) CurrentBlock() (uint64, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return 0, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	header, err := ethcli.HeaderByNumber(timeout, nil)
	if err != nil {
		return 0, err
	}
	return header.Number.Uint64(), nil
}

func (onr *OneNodeReader) StorageAt(atBlock int64, contractAddr string, slot string) ([]byte, error) {
	ethcli, err := onr.EthClient()
	if err != nil {
		return []byte{}, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	contract := jarviscommon.HexToAddress(contractAddr)
	hash := common.HexToHash(slot)
	var blockBig *big.Int
	if atBlock > 0 {
		blockBig = big.NewInt(atBlock)
	}

	return ethcli.StorageAt(timeout, contract, hash, blockBig)
}
