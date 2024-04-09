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
	"github.com/ethereum/go-ethereum/rpc"

	. "github.com/tranvictor/jarvis/common"
)

const TIMEOUT time.Duration = 4 * time.Second

type OneNodeReader struct {
	nodeName  string
	nodeURL   string
	client    *rpc.Client
	ethClient *ethclient.Client
	mu        sync.Mutex
}

func NewOneNodeReader(name, url string) *OneNodeReader {
	return &OneNodeReader{
		nodeName:  name,
		nodeURL:   url,
		client:    nil,
		ethClient: nil,
		mu:        sync.Mutex{},
	}
}

func (self *OneNodeReader) NodeName() string {
	return self.nodeName
}

func (self *OneNodeReader) NodeURL() string {
	return self.nodeURL
}

func (self *OneNodeReader) initConnection() error {
	self.mu.Lock()
	defer self.mu.Unlock()
	client, err := rpc.Dial(self.NodeURL())
	if err != nil {
		return fmt.Errorf("Couldn't connect to %s: %w", self.nodeName, err)
	}
	self.client = client
	self.ethClient = ethclient.NewClient(self.client)
	return nil
}

func (self *OneNodeReader) Client() (*rpc.Client, error) {
	if self.client != nil {
		return self.client, nil
	}
	err := self.initConnection()
	return self.client, err
}

func (self *OneNodeReader) EthClient() (*ethclient.Client, error) {
	if self.ethClient != nil {
		return self.ethClient, nil
	}
	err := self.initConnection()
	return self.ethClient, err
}

func (self *OneNodeReader) EstimateGas(from, to string, priceGwei float64, value *big.Int, data []byte) (uint64, error) {
	fromAddr := common.HexToAddress(from)
	var toAddrPtr *common.Address
	if to != "" {
		toAddr := common.HexToAddress(to)
		toAddrPtr = &toAddr
	}
	price := FloatToBigInt(priceGwei, 9)
	ethcli, err := self.EthClient()
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

func (self *OneNodeReader) GetCode(address string) (code []byte, err error) {
	addr := common.HexToAddress(address)
	ethcli, err := self.EthClient()
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.CodeAt(timeout, addr, nil)
}

func (self *OneNodeReader) GetGasPriceSuggestion() (*big.Int, error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return nil, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.SuggestGasPrice(timeout)
}

func (self *OneNodeReader) GetBalance(address string) (balance *big.Int, err error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return nil, err
	}
	acc := common.HexToAddress(address)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.BalanceAt(timeout, acc, nil)
}

func (self *OneNodeReader) GetMinedNonce(address string) (nonce uint64, err error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return 0, err
	}
	acc := common.HexToAddress(address)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.NonceAt(timeout, acc, nil)
}

func (self *OneNodeReader) GetPendingNonce(address string) (nonce uint64, err error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return 0, err
	}
	acc := common.HexToAddress(address)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.PendingNonceAt(timeout, acc)
}

func (self *OneNodeReader) TransactionReceipt(txHash string) (receipt *types.Receipt, err error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return nil, err
	}
	hash := common.HexToHash(txHash)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.TransactionReceipt(timeout, hash)
}

func (self *OneNodeReader) transactionByHashOnNode(ctx context.Context, hash common.Hash, client *rpc.Client) (tx *Transaction, isPending bool, err error) {
	var json *Transaction
	cli, err := self.Client()
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

func (self *OneNodeReader) TransactionByHash(txHash string) (tx *Transaction, isPending bool, err error) {
	cli, err := self.Client()
	if err != nil {
		return nil, false, err
	}

	hash := common.HexToHash(txHash)
	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return self.transactionByHashOnNode(timeout, hash, cli)
}

// func (self *OneNodeReader) Call(result interface{}, method string, args ...interface{}) error {
// 	cli, err := self.Client()
// 	if err != nil {
// 		return err
// 	}
// 	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
// 	defer cancel()
// 	return cli.CallContext(timeout, result, method, args)
// }

func (self *OneNodeReader) HeaderByNumber(number int64) (*types.Header, error) {
	ethcli, err := self.EthClient()
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

func (self *OneNodeReader) SuggestedGasTipCap() (*big.Int, error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return nil, err
	}

	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	return ethcli.SuggestGasTipCap(timeout)
}

func (self *OneNodeReader) GetLogs(fromBlock, toBlock int, addresses []string, topic string) ([]types.Log, error) {
	ethcli, err := self.EthClient()
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
	q.Addresses = HexToAddresses(addresses)
	q.Topics = [][]common.Hash{
		{HexToHash(topic)},
	}

	timeout, cancel := context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()
	return ethcli.FilterLogs(timeout, *q)
}

func (self *OneNodeReader) ReadContractToBytes(atBlock int64, from string, caddr string, abi *abi.ABI, method string, args ...interface{}) ([]byte, error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return nil, err
	}

	contract := HexToAddress(caddr)
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
		From:     HexToAddress(from),
		To:       &contract,
		Gas:      0,
		GasPrice: nil,
		Value:    nil,
		Data:     data,
	}, blockBig)
}

func (self *OneNodeReader) CurrentBlock() (uint64, error) {
	ethcli, err := self.EthClient()
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

func (self *OneNodeReader) StorageAt(atBlock int64, contractAddr string, slot string) ([]byte, error) {
	ethcli, err := self.EthClient()
	if err != nil {
		return []byte{}, err
	}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	contract := HexToAddress(contractAddr)
	hash := common.HexToHash(slot)
	var blockBig *big.Int
	if atBlock > 0 {
		blockBig = big.NewInt(atBlock)
	}

	return ethcli.StorageAt(timeout, contract, hash, blockBig)
}
