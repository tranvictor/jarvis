package reader

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"

	jarviscommon "github.com/tranvictor/jarvis/common"
	jarvisnetworks "github.com/tranvictor/jarvis/util/explorers"
)

var DEFAULT_ADDRESS string = "0x0000000000000000000000000000000000000000"

const (
	DEFAULT_ETHERSCAN_APIKEY string = "UBB257TI824FC7HUSPT66KZUMGBPRN3IWV"
	DEFAULT_BSCSCAN_APIKEY   string = "62TU8Z81F7ESNJT38ZVRBSX7CNN4QZSP5I"
	DEFAULT_TOMOSCAN_APIKEY  string = ""
)

type EthReader struct {
	nodes map[string]EthereumNode
	be    jarvisnetworks.BlockExplorer
}

func NewEthReaderGeneric(nodes map[string]string, be jarvisnetworks.BlockExplorer) *EthReader {
	ns := map[string]EthereumNode{}
	for name, c := range nodes {
		ns[name] = NewOneNodeReader(name, c)
	}
	return &EthReader{
		nodes: ns,
		be:    be,
	}
}

func wrapError(e error, name string) error {
	if e == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", name, e)
}

type estimateGasResult struct {
	Gas   uint64
	Error error
}

func (er *EthReader) EstimateExactGas(
	from, to string,
	priceGwei float64,
	value *big.Int,
	data []byte,
) (uint64, error) {
	resCh := make(chan estimateGasResult, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			// passing negative atBlock param to estimate gas on pending block
			gas, err := n.EstimateGas(from, to, priceGwei, value, data, big.NewInt(-1))
			resCh <- estimateGasResult{
				Gas:   gas,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Gas, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) EstimateGas(
	from, to string,
	priceGwei, value float64,
	data []byte,
) (uint64, error) {
	return er.EstimateExactGas(from, to, priceGwei, jarviscommon.FloatToBigInt(value, 18), data)
}

type getCodeResponse struct {
	Code  []byte
	Error error
}

func (er *EthReader) GetCode(address string) (code []byte, err error) {
	resCh := make(chan getCodeResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			code, err := n.GetCode(address)
			resCh <- getCodeResponse{
				Code:  code,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Code, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) TxInfoFromHash(tx string) (jarviscommon.TxInfo, error) {
	txObj, isPending, err := er.TransactionByHash(tx)

	if err != nil {
		return jarviscommon.TxInfo{
			Status:      "error",
			Tx:          nil,
			InternalTxs: []jarviscommon.InternalTx{},
			Receipt:     nil,
		}, err
	}
	if txObj == nil {
		return jarviscommon.TxInfo{
			Status:      "notfound",
			Tx:          nil,
			InternalTxs: []jarviscommon.InternalTx{},
			Receipt:     nil,
		}, nil
	}
	if isPending {
		return jarviscommon.TxInfo{
			Status:      "pending",
			Tx:          txObj,
			InternalTxs: []jarviscommon.InternalTx{},
			Receipt:     nil,
		}, nil
	}

	receipt, err := er.TransactionReceipt(tx)

	if receipt == nil {
		return jarviscommon.TxInfo{
			Status:      "pending",
			Tx:          txObj,
			InternalTxs: []jarviscommon.InternalTx{},
			Receipt:     nil,
		}, err
	}

	// block, _ := er.HeaderByNumber(receipt.BlockNumber.Int64())
	// only byzantium has status field at the moment
	// mainnet, ropsten are byzantium, other chains such as
	// devchain, kovan are not.
	// if PostState is a hash, it is pre-byzantium and all
	// txs with PostState are considered done
	if len(receipt.PostState) == len(common.Hash{}) {
		return jarviscommon.TxInfo{
			Status:      "done",
			Tx:          txObj,
			InternalTxs: []jarviscommon.InternalTx{},
			Receipt:     receipt,
		}, nil
	} else {
		if receipt.Status == 1 {
			// successful tx
			return jarviscommon.TxInfo{
				Status:      "done",
				Tx:          txObj,
				InternalTxs: []jarviscommon.InternalTx{},
				Receipt:     receipt,
			}, nil
		}
		// failed tx
		return jarviscommon.TxInfo{
			Status:      "reverted",
			Tx:          txObj,
			InternalTxs: []jarviscommon.InternalTx{},
			Receipt:     receipt,
		}, nil
	}
}

type getGasSuggestionResponse struct {
	GasPrice *big.Int
	Error    error
}

func (er *EthReader) GetGasPriceWeiSuggestion() (*big.Int, error) {
	resCh := make(chan getGasSuggestionResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			price, err := n.GetGasPriceSuggestion()
			resCh <- getGasSuggestionResponse{
				GasPrice: price,
				Error:    wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.GasPrice, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

type getBalanceResponse struct {
	Balance *big.Int
	Error   error
}

func (er *EthReader) GetBalance(address string) (balance *big.Int, err error) {
	resCh := make(chan getBalanceResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			balance, err := n.GetBalance(address)
			resCh <- getBalanceResponse{
				Balance: balance,
				Error:   wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Balance, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

type getNonceResponse struct {
	Nonce uint64
	Error error
}

func (er *EthReader) GetMinedNonce(address string) (nonce uint64, err error) {
	resCh := make(chan getNonceResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			nonce, err := n.GetMinedNonce(address)
			resCh <- getNonceResponse{
				Nonce: nonce,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Nonce, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) GetPendingNonce(address string) (nonce uint64, err error) {
	resCh := make(chan getNonceResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			nonce, err := n.GetPendingNonce(address)
			resCh <- getNonceResponse{
				Nonce: nonce,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Nonce, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

type transactionReceiptResponse struct {
	Receipt *types.Receipt
	Error   error
}

func (er *EthReader) TransactionReceipt(txHash string) (receipt *types.Receipt, err error) {
	resCh := make(chan transactionReceiptResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			receipt, err := n.TransactionReceipt(txHash)
			resCh <- transactionReceiptResponse{
				Receipt: receipt,
				Error:   wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Receipt, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

type transactionByHashResponse struct {
	Tx        *jarviscommon.Transaction
	IsPending bool
	Error     error
}

func (er *EthReader) TransactionByHash(
	txHash string,
) (tx *jarviscommon.Transaction, isPending bool, err error) {
	resCh := make(chan transactionByHashResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
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
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Tx, result.IsPending, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, false, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

type readContractToBytesResponse struct {
	Data  []byte
	Error error
}

func (er *EthReader) ReadContractToBytes(
	atBlock int64,
	from string,
	caddr string,
	abi *abi.ABI,
	method string,
	args ...interface{},
) ([]byte, error) {
	resCh := make(chan readContractToBytesResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			data, err := n.ReadContractToBytes(atBlock, from, caddr, abi, method, args...)
			resCh <- readContractToBytesResponse{
				Data:  data,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Data, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) EthCall(from string, to string, data []byte, overrides *map[common.Address]gethclient.OverrideAccount) ([]byte, error) {
	resCh := make(chan readContractToBytesResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			data, err := n.EthCall(from, to, data, overrides)
			resCh <- readContractToBytesResponse{
				Data:  data,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Data, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) ImplementationOfEIP1967(
	atBlock int64,
	caddr string,
) (common.Address, error) {
	// eip 1967
	// bytes32(uint256(keccak256('eip1967.proxy.implementation')) - 1)
	slotBig := big.NewInt(0).Sub(
		crypto.Keccak256Hash([]byte("eip1967.proxy.implementation")).Big(),
		big.NewInt(1),
	)

	addrByte, err := er.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
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

	addrByte, err = er.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
	if err != nil {
		return common.Address{}, err
	}

	beaconAddr := common.BytesToAddress(addrByte)

	if beaconAddr.Big().Cmp(big.NewInt(0)) != 0 {
		paddr, err := er.AddressFromContractWithABI(
			beaconAddr.Hex(),
			jarviscommon.GetEIP1967BeaconABI(),
			"implementation",
		)
		return *paddr, err
	}

	return common.Address{}, fmt.Errorf("not an eip1967 proxy contract")
}

func (er *EthReader) ImplementationOf(atBlock int64, caddr string) (common.Address, error) {
	addr, err := er.ImplementationOfEIP1967(atBlock, caddr)
	if err == nil {
		return addr, nil
	}

	// old standard: org.zeppelinos.proxy.implementation
	slotBig := crypto.Keccak256Hash([]byte("org.zeppelinos.proxy.implementation")).Big()

	addrByte, err := er.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
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

	addrByte, err = er.StorageAt(atBlock, caddr, common.BigToHash(slotBig).Hex())
	if err != nil {
		return common.Address{}, err
	}

	return common.BytesToAddress(addrByte), nil
}

func (er *EthReader) StorageAt(atBlock int64, caddr string, slot string) ([]byte, error) {
	resCh := make(chan readContractToBytesResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			data, err := n.StorageAt(atBlock, caddr, slot)
			resCh <- readContractToBytesResponse{
				Data:  data,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Data, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) ReadHistoryContractWithABI(
	atBlock int64,
	result interface{},
	caddr string,
	abi *abi.ABI,
	method string,
	args ...interface{},
) error {
	responseBytes, err := er.ReadContractToBytes(
		int64(atBlock), DEFAULT_ADDRESS, caddr, abi, method, args...)
	if err != nil {
		return err
	}
	return abi.UnpackIntoInterface(result, method, responseBytes)
}

func (er *EthReader) ReadContractWithABIAndFrom(
	result interface{},
	from string,
	caddr string,
	abi *abi.ABI,
	method string,
	args ...interface{},
) error {
	responseBytes, err := er.ReadContractToBytes(-1, from, caddr, abi, method, args...)
	if err != nil {
		return err
	}
	return abi.UnpackIntoInterface(result, method, responseBytes)
}

func (er *EthReader) ReadContractWithABI(
	result interface{},
	caddr string,
	abi *abi.ABI,
	method string,
	args ...interface{},
) error {
	responseBytes, err := er.ReadContractToBytes(-1, DEFAULT_ADDRESS, caddr, abi, method, args...)
	if err != nil {
		return err
	}
	return abi.UnpackIntoInterface(result, method, responseBytes)
}

func (er *EthReader) ReadHistoryContract(
	atBlock int64,
	result interface{},
	caddr string,
	method string,
	args ...interface{},
) error {
	abi, err := er.GetABI(caddr)
	if err != nil {
		return err
	}
	return er.ReadHistoryContractWithABI(atBlock, result, caddr, abi, method, args...)
}

func (er *EthReader) ReadContract(
	result interface{},
	caddr string,
	method string,
	args ...interface{},
) error {
	abi, err := er.GetABI(caddr)
	if err != nil {
		return err
	}
	return er.ReadContractWithABI(result, caddr, abi, method, args...)
}

func (er *EthReader) HistoryERC20Balance(
	atBlock int64,
	caddr string,
	user string,
) (*big.Int, error) {
	abi := jarviscommon.GetERC20ABI()
	result := big.NewInt(0)
	err := er.ReadHistoryContractWithABI(
		atBlock,
		&result,
		caddr,
		abi,
		"balanceOf",
		jarviscommon.HexToAddress(user),
	)
	return result, err
}

func (er *EthReader) ERC20Symbol(caddr string) (string, error) {
	abi := jarviscommon.GetERC20ABI()
	var result string
	err := er.ReadContractWithABI(&result, caddr, abi, "symbol")
	return result, err
}

func (er *EthReader) ERC20Balance(caddr string, user string) (*big.Int, error) {
	abi := jarviscommon.GetERC20ABI()
	result := big.NewInt(0)
	err := er.ReadContractWithABI(&result, caddr, abi, "balanceOf", jarviscommon.HexToAddress(user))
	return result, err
}

func (er *EthReader) HistoryERC20Decimal(atBlock int64, caddr string) (int64, error) {
	abi := jarviscommon.GetERC20ABI()
	var result uint8
	err := er.ReadHistoryContractWithABI(atBlock, &result, caddr, abi, "decimals")
	return int64(result), err
}

func (er *EthReader) ERC20Decimal(caddr string) (uint64, error) {
	abi := jarviscommon.GetERC20ABI()
	var result uint8
	err := er.ReadContractWithABI(&result, caddr, abi, "decimals")
	return uint64(result), err
}

type headerByNumberResponse struct {
	Header *types.Header
	Error  error
}

func (er *EthReader) HeaderByNumber(number int64) (*types.Header, error) {
	resCh := make(chan headerByNumberResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			header, err := n.HeaderByNumber(number)
			resCh <- headerByNumberResponse{
				Header: header,
				Error:  wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Header, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) SuggestedGasSettings() (maxGasPriceGwei, maxTipGwei float64, err error) {
	isDynamicFeeAvailable, err := er.CheckDynamicFeeTxAvailable()
	if err != nil {
		return 0, 0, err
	}

	maxGasPriceGwei, err = er.RecommendedGasPrice()
	if err != nil {
		return 0, 0, err
	}

	if isDynamicFeeAvailable {
		maxTipGwei, err = er.GetSuggestedGasTipCap()
		if err != nil {
			return 0, 0, err
		}
	}

	return maxGasPriceGwei, maxTipGwei, nil
}

// CheckDynamicFeeTxAvailable use to detect if current network that connect via node url is support dynamic fee tx,
// this is done by a trick where we check if block info contain baseFee > 0, that may not always work but should enough
// for now.
func (er *EthReader) CheckDynamicFeeTxAvailable() (bool, error) {
	header, err := er.HeaderByNumber(-1) // getting latest block header
	if err != nil {
		return false, err
	}

	return header.BaseFee != nil && header.BaseFee.Cmp(common.Big0) > 0, nil
}

type getSuggestedGasResponse struct {
	Gas   *big.Int
	Error error
}

// add 20% tip to miners compared to what returned from the node to improve UX
// a bit more
func (er *EthReader) GetSuggestedGasTipCap() (float64, error) {
	resCh := make(chan getSuggestedGasResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			gasTip, err := n.SuggestedGasTipCap()
			resCh <- getSuggestedGasResponse{
				Gas:   gasTip,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}

	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return jarviscommon.BigToFloat(result.Gas, 9) * 1.2, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

// add 50% to max gas price because the next blocks based price can be increased
// according to ethereum protocol
func (er *EthReader) RecommendedGasPrice() (float64, error) {
	resCh := make(chan getSuggestedGasResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			gasTip, err := n.SuggestedGasPrice()
			resCh <- getSuggestedGasResponse{
				Gas:   gasTip,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}

	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return jarviscommon.BigToFloat(result.Gas, 9) * 1.5, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) HistoryERC20Allowance(
	atBlock int64,
	caddr string,
	owner string,
	spender string,
) (*big.Int, error) {
	abi := jarviscommon.GetERC20ABI()
	result := big.NewInt(0)
	err := er.ReadHistoryContractWithABI(
		atBlock,
		&result, caddr, abi,
		"allowance",
		jarviscommon.HexToAddress(owner),
		jarviscommon.HexToAddress(spender),
	)
	return result, err
}

func (er *EthReader) ERC20Allowance(
	caddr string,
	owner string,
	spender string,
) (*big.Int, error) {
	abi := jarviscommon.GetERC20ABI()
	result := big.NewInt(0)
	err := er.ReadContractWithABI(
		&result, caddr, abi,
		"allowance",
		jarviscommon.HexToAddress(owner),
		jarviscommon.HexToAddress(spender),
	)
	return result, err
}

func (er *EthReader) AddressFromContractWithABI(
	contract string,
	abi *abi.ABI,
	method string,
) (*common.Address, error) {
	result := common.Address{}
	err := er.ReadContractWithABI(&result, contract, abi, method)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (er *EthReader) AddressFromContract(
	contract string,
	method string,
) (*common.Address, error) {
	result := common.Address{}
	err := er.ReadContract(&result, contract, method)
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
func (er *EthReader) GetLogs(
	fromBlock, toBlock int,
	addresses []string,
	topic string,
) ([]types.Log, error) {
	resCh := make(chan getLogsResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			logs, err := n.GetLogs(fromBlock, toBlock, addresses, topic)
			resCh <- getLogsResponse{
				Logs:  logs,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Logs, result.Error
		}
		errs = append(errs, result.Error)
	}
	return nil, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

type getBlockResponse struct {
	Block uint64
	Error error
}

func (er *EthReader) CurrentBlock() (uint64, error) {
	resCh := make(chan getBlockResponse, len(er.nodes))
	for i := range er.nodes {
		n := er.nodes[i]
		go func() {
			block, err := n.CurrentBlock()
			resCh <- getBlockResponse{
				Block: block,
				Error: wrapError(err, n.NodeName()),
			}
		}()
	}
	errs := []error{}
	for i := 0; i < len(er.nodes); i++ {
		result := <-resCh
		if result.Error == nil {
			return result.Block, result.Error
		}
		errs = append(errs, result.Error)
	}
	return 0, fmt.Errorf("couldn't read from any nodes: %w", errors.Join(errs...))
}

func (er *EthReader) GetABIString(address string) (string, error) {
	return er.be.GetABIString(address)
}

func (er *EthReader) GetABI(address string) (*abi.ABI, error) {
	body, err := er.GetABIString(address)
	if err != nil {
		return nil, err
	}

	result, err := abi.JSON(strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	return &result, nil
}
