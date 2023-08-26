package reader

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	. "github.com/tranvictor/jarvis/common"
	. "github.com/tranvictor/jarvis/util/explorers"
)

var (
	DEFAULT_ADDRESS string = "0x0000000000000000000000000000000000000000"
)

const (
	DEFAULT_ETHERSCAN_APIKEY string = "UBB257TI824FC7HUSPT66KZUMGBPRN3IWV"
	DEFAULT_BSCSCAN_APIKEY   string = "62TU8Z81F7ESNJT38ZVRBSX7CNN4QZSP5I"
	DEFAULT_TOMOSCAN_APIKEY  string = ""
)

type EthReader struct {
	nodes map[string]EthereumNode
	be    BlockExplorer
}

func NewEthReaderGeneric(nodes map[string]string, be BlockExplorer) *EthReader {
	ns := map[string]EthereumNode{}
	for name, c := range nodes {
		ns[name] = NewOneNodeReader(name, c)
	}
	return &EthReader{
		nodes: ns,
		be:    be,
	}
}

func errorInfo(errs []error) string {
	estrs := []string{}
	for i, e := range errs {
		estrs = append(estrs, fmt.Sprintf("%d. %s", i+1, e))
	}
	return strings.Join(estrs, "\n")
}

func wrapError(e error, name string) error {
	if e == nil {
		return nil
	}
	return fmt.Errorf("%s: %s", name, e)
}

type estimateGasResult struct {
	Gas   uint64
	Error error
}

func (self *EthReader) EstimateExactGas(from, to string, priceGwei float64, value *big.Int, data []byte) (uint64, error) {
	resCh := make(chan estimateGasResult, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			gas, err := n.EstimateGas(from, to, priceGwei, value, data)
			resCh <- estimateGasResult{
				Gas:   gas,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Gas, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) EstimateGas(from, to string, priceGwei, value float64, data []byte) (uint64, error) {
	return self.EstimateExactGas(from, to, priceGwei, FloatToBigInt(value, 18), data)
}

type getCodeResponse struct {
	Code  []byte
	Error error
}

func (self *EthReader) GetCode(address string) (code []byte, err error) {
	resCh := make(chan getCodeResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			code, err := n.GetCode(address)
			resCh <- getCodeResponse{
				Code:  code,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Code, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) TxInfoFromHash(tx string) (TxInfo, error) {
	txObj, isPending, err := self.TransactionByHash(tx)
	if err != nil {
		return TxInfo{"error", nil, nil, nil}, err
	}
	if txObj == nil {
		return TxInfo{"notfound", nil, nil, nil}, nil
	} else {
		if isPending {
			return TxInfo{"pending", txObj, nil, nil}, nil
		} else {
			receipt, err := self.TransactionReceipt(tx)
			if receipt == nil {
				return TxInfo{"pending", txObj, nil, nil}, err
			} else {
				// block, _ := self.HeaderByNumber(receipt.BlockNumber.Int64())
				// only byzantium has status field at the moment
				// mainnet, ropsten are byzantium, other chains such as
				// devchain, kovan are not.
				// if PostState is a hash, it is pre-byzantium and all
				// txs with PostState are considered done
				if len(receipt.PostState) == len(common.Hash{}) {
					return TxInfo{"done", txObj, []InternalTx{}, receipt}, nil
				} else {
					if receipt.Status == 1 {
						// successful tx
						return TxInfo{"done", txObj, []InternalTx{}, receipt}, nil
					}
					// failed tx
					return TxInfo{"reverted", txObj, []InternalTx{}, receipt}, nil
				}
			}
		}
	}
}

type ksresponse struct {
	Data struct {
		Fast     string
		Standard string
		Low      string
		Default  string
	}
	Success bool
}

func (self *EthReader) RecommendedGasPriceFromKyberSwap() (low, average, fast float64, err error) {
	resp, err := http.Get("https://production-cache.kyber.network/gasPrice")
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0, err
	}
	prices := ksresponse{}
	err = json.Unmarshal(body, &prices)
	if err != nil {
		return 0, 0, 0, err
	}
	if !prices.Success {
		return 0, 0, 0, fmt.Errorf("failed response from kyberswap")
	}

	fastFloat, err := strconv.ParseFloat(prices.Data.Fast, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	standardFloat, err := strconv.ParseFloat(prices.Data.Standard, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	lowFloat, err := strconv.ParseFloat(prices.Data.Low, 64)
	if err != nil {
		return 0, 0, 0, err
	}

	return lowFloat, standardFloat, fastFloat, nil
}

// gas station response
type gsresponse struct {
	Average float64 `json:"average"`
	Fast    float64 `json:"fast"`
	Fastest float64 `json:"fastest"`
	SafeLow float64 `json:"safeLow"`
}

// func (self *EthReader) RecommendedGasPriceFromEthGasStation(link string) (low, average, fast float64, err error) {
// 	resp, err := http.Get(link)
// 	if err != nil {
// 		return 0, 0, 0, err
// 	}
// 	defer resp.Body.Close()
// 	body, err := ioutil.ReadAll(resp.Body)
// 	if err != nil {
// 		return 0, 0, 0, err
// 	}
// 	prices := gsresponse{}
// 	err = json.Unmarshal(body, &prices)
// 	if err != nil {
// 		return 0, 0, 0, err
// 	}
// 	return prices.SafeLow / 10, prices.Average / 10, prices.Fast / 10, nil
// }

// return gwei
func (self *EthReader) RecommendedGasPrice() (float64, error) {
	price, err := self.be.RecommendedGasPrice()
	if err != nil {
		priceWei, err := self.GetGasPriceWeiSuggestion()
		if err != nil {
			return 0, err
		}
		return BigToFloat(priceWei, 9), nil
	}
	return price, nil
}

type getGasSuggestionResponse struct {
	GasPrice *big.Int
	Error    error
}

func (self *EthReader) GetGasPriceWeiSuggestion() (*big.Int, error) {
	resCh := make(chan getGasSuggestionResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			price, err := n.GetGasPriceSuggestion()
			resCh <- getGasSuggestionResponse{
				GasPrice: price,
				Error:    wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.GasPrice, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

type getBalanceResponse struct {
	Balance *big.Int
	Error   error
}

func (self *EthReader) GetBalance(address string) (balance *big.Int, err error) {
	resCh := make(chan getBalanceResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			balance, err := n.GetBalance(address)
			resCh <- getBalanceResponse{
				Balance: balance,
				Error:   wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Balance, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

type getNonceResponse struct {
	Nonce uint64
	Error error
}

func (self *EthReader) GetMinedNonce(address string) (nonce uint64, err error) {
	resCh := make(chan getNonceResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			nonce, err := n.GetMinedNonce(address)
			resCh <- getNonceResponse{
				Nonce: nonce,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Nonce, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) GetPendingNonce(address string) (nonce uint64, err error) {
	resCh := make(chan getNonceResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			nonce, err := n.GetPendingNonce(address)
			resCh <- getNonceResponse{
				Nonce: nonce,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Nonce, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

type transactionReceiptResponse struct {
	Receipt *types.Receipt
	Error   error
}

func (self *EthReader) TransactionReceipt(txHash string) (receipt *types.Receipt, err error) {
	resCh := make(chan transactionReceiptResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			receipt, err := n.TransactionReceipt(txHash)
			resCh <- transactionReceiptResponse{
				Receipt: receipt,
				Error:   wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Receipt, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

type transactionByHashResponse struct {
	Tx        *Transaction
	IsPending bool
	Error     error
}

func (self *EthReader) TransactionByHash(txHash string) (tx *Transaction, isPending bool, err error) {
	resCh := make(chan transactionByHashResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			tx, ispending, err := n.TransactionByHash(txHash)
			resCh <- transactionByHashResponse{
				Tx:        tx,
				IsPending: ispending,
				Error:     wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Tx, result.IsPending, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, false, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

type readContractToBytesResponse struct {
	Data  []byte
	Error error
}

func (self *EthReader) ReadContractToBytes(atBlock int64, from string, caddr string, abi *abi.ABI, method string, args ...interface{}) ([]byte, error) {
	resCh := make(chan readContractToBytesResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			data, err := n.ReadContractToBytes(atBlock, from, caddr, abi, method, args...)
			resCh <- readContractToBytesResponse{
				Data:  data,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Data, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) ImplementationOfEIP1967(atBlock int64, caddr string) (common.Address, error) {
	// eip 1967
	// bytes32(uint256(keccak256('eip1967.proxy.implementation')) - 1)
	slotBig := big.NewInt(0).Sub(
		crypto.Keccak256Hash([]byte("eip1967.proxy.implementation")).Big(),
		big.NewInt(1),
	)

	addrByte, err := self.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
	if err != nil {
		return common.Address{}, err
	}

	addr := common.BytesToAddress(addrByte)

	if addr.Big().Cmp(big.NewInt(0)) != 0 {
		return addr, nil
	}

	// eip 1967
	// bytes32(uint256(keccak256('eip1967.proxy.beacon')) - 1)
	slotBig = big.NewInt(0).Sub(
		crypto.Keccak256Hash([]byte("eip1967.proxy.beacon")).Big(),
		big.NewInt(1),
	)

	addrByte, err = self.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
	if err != nil {
		return common.Address{}, err
	}

  beaconAddr := common.BytesToAddress(addrByte)

	if beaconAddr.Big().Cmp(big.NewInt(0)) != 0 {
    paddr, err := self.AddressFromContractWithABI(beaconAddr.Hex(), GetEIP1967BeaconABI(), "implementation")
    return *paddr, err
	}

  return common.Address{}, fmt.Errorf("not an eip1967 proxy contract")
}

func (self *EthReader) ImplementationOf(atBlock int64, caddr string) (common.Address, error) {
  addr, err := self.ImplementationOfEIP1967(atBlock, caddr)
  if err == nil {
    return addr, nil
  }

	// old standard: org.zeppelinos.proxy.implementation
  slotBig := crypto.Keccak256Hash([]byte("org.zeppelinos.proxy.implementation")).Big()

  addrByte, err := self.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
	if err != nil {
		return common.Address{}, err
	}

	addr = common.BytesToAddress(addrByte)

	if addr.Big().Cmp(big.NewInt(0)) != 0 {
		return addr, nil
	}

	// eip 1967 on Poygon
	// bytes32(uint256(keccak256('matic.network.proxy.implementation')) - 1)
	slotBig = big.NewInt(0).Sub(
		crypto.Keccak256Hash([]byte("matic.network.proxy.implementation")).Big(),
		big.NewInt(1),
	)

	addrByte, err = self.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
	if err != nil {
		return common.Address{}, err
	}

	return common.BytesToAddress(addrByte), nil
}

func (self *EthReader) StorageAt(atBlock int64, caddr string, slot string) ([]byte, error) {
	resCh := make(chan readContractToBytesResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			data, err := n.StorageAt(atBlock, caddr, slot)
			resCh <- readContractToBytesResponse{
				Data:  data,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Data, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) ReadHistoryContractWithABI(atBlock int64, result interface{}, caddr string, abi *abi.ABI, method string, args ...interface{}) error {
	responseBytes, err := self.ReadContractToBytes(
		int64(atBlock), DEFAULT_ADDRESS, caddr, abi, method, args...)
	if err != nil {
		return err
	}
	return abi.UnpackIntoInterface(result, method, responseBytes)
}

func (self *EthReader) ReadContractWithABIAndFrom(result interface{}, from string, caddr string, abi *abi.ABI, method string, args ...interface{}) error {
	responseBytes, err := self.ReadContractToBytes(-1, from, caddr, abi, method, args...)
	if err != nil {
		return err
	}
	return abi.UnpackIntoInterface(result, method, responseBytes)
}

func (self *EthReader) ReadContractWithABI(result interface{}, caddr string, abi *abi.ABI, method string, args ...interface{}) error {
	responseBytes, err := self.ReadContractToBytes(-1, DEFAULT_ADDRESS, caddr, abi, method, args...)
	if err != nil {
		return err
	}
	return abi.UnpackIntoInterface(result, method, responseBytes)
}

func (self *EthReader) ReadHistoryContract(atBlock int64, result interface{}, caddr string, method string, args ...interface{}) error {
	abi, err := self.GetABI(caddr)
	if err != nil {
		return err
	}
	return self.ReadHistoryContractWithABI(atBlock, result, caddr, abi, method, args...)
}

func (self *EthReader) ReadContract(result interface{}, caddr string, method string, args ...interface{}) error {
	abi, err := self.GetABI(caddr)
	if err != nil {
		return err
	}
	return self.ReadContractWithABI(result, caddr, abi, method, args...)
}

func (self *EthReader) HistoryERC20Balance(atBlock int64, caddr string, user string) (*big.Int, error) {
	abi := GetERC20ABI()
	result := big.NewInt(0)
	err := self.ReadHistoryContractWithABI(atBlock, &result, caddr, abi, "balanceOf", HexToAddress(user))
	return result, err
}

func (self *EthReader) ERC20Symbol(caddr string) (string, error) {
	abi := GetERC20ABI()
	var result string
	err := self.ReadContractWithABI(&result, caddr, abi, "symbol")
	return result, err
}

func (self *EthReader) ERC20Balance(caddr string, user string) (*big.Int, error) {
	abi := GetERC20ABI()
	result := big.NewInt(0)
	err := self.ReadContractWithABI(&result, caddr, abi, "balanceOf", HexToAddress(user))
	return result, err
}

func (self *EthReader) HistoryERC20Decimal(atBlock int64, caddr string) (int64, error) {
	abi := GetERC20ABI()
	var result uint8
	err := self.ReadHistoryContractWithABI(atBlock, &result, caddr, abi, "decimals")
	return int64(result), err
}

func (self *EthReader) ERC20Decimal(caddr string) (uint64, error) {
	abi := GetERC20ABI()
	var result uint8
	err := self.ReadContractWithABI(&result, caddr, abi, "decimals")
	return uint64(result), err
}

type headerByNumberResponse struct {
	Header *types.Header
	Error  error
}

func (self *EthReader) HeaderByNumber(number int64) (*types.Header, error) {
	resCh := make(chan headerByNumberResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			header, err := n.HeaderByNumber(number)
			resCh <- headerByNumberResponse{
				Header: header,
				Error:  wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Header, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) HistoryERC20Allowance(atBlock int64, caddr string, owner string, spender string) (*big.Int, error) {
	abi := GetERC20ABI()
	result := big.NewInt(0)
	err := self.ReadHistoryContractWithABI(
		atBlock,
		&result, caddr, abi,
		"allowance",
		HexToAddress(owner),
		HexToAddress(spender),
	)
	return result, err
}

func (self *EthReader) ERC20Allowance(caddr string, owner string, spender string) (*big.Int, error) {
	abi := GetERC20ABI()
	result := big.NewInt(0)
	err := self.ReadContractWithABI(
		&result, caddr, abi,
		"allowance",
		HexToAddress(owner),
		HexToAddress(spender),
	)
	return result, err
}

func (self *EthReader) AddressFromContractWithABI(contract string, abi *abi.ABI, method string) (*common.Address, error) {
	result := common.Address{}
	err := self.ReadContractWithABI(&result, contract, abi, method)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (self *EthReader) AddressFromContract(contract string, method string) (*common.Address, error) {
	result := common.Address{}
	err := self.ReadContract(&result, contract, method)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

type getLogsResponse struct {
	Logs  []types.Log
	Error error
}

// if toBlock < 0, it will query to the latest block
func (self *EthReader) GetLogs(fromBlock, toBlock int, addresses []string, topic string) ([]types.Log, error) {
	resCh := make(chan getLogsResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			logs, err := n.GetLogs(fromBlock, toBlock, addresses, topic)
			resCh <- getLogsResponse{
				Logs:  logs,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Logs, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

type getBlockResponse struct {
	Block uint64
	Error error
}

func (self *EthReader) CurrentBlock() (uint64, error) {
	resCh := make(chan getBlockResponse, len(self.nodes))
	for i, _ := range self.nodes {
		n := self.nodes[i]
		go func() {
			block, err := n.CurrentBlock()
			resCh <- getBlockResponse{
				Block: block,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(self.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Block, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("Couldn't read from any nodes: %s", errorInfo(errs))
}

func (self *EthReader) GetABIString(address string) (string, error) {
	return self.be.GetABIString(address)
}

func (self *EthReader) GetABI(address string) (*abi.ABI, error) {
	body, err := self.GetABIString(address)
	if err != nil {
		return nil, err
	}

	result, err := abi.JSON(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	return &result, nil
}
