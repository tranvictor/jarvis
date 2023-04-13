package account

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/trezoreum"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/reader"
)

type Account struct {
	signer      Signer
	reader      *reader.EthReader
	broadcaster *broadcaster.Broadcaster
	address     common.Address
}

func NewKeystoreAccountGeneric(file string, password string, reader *reader.EthReader, broadcaster *broadcaster.Broadcaster, chainID int64) (*Account, error) {
	_, key, err := PrivateKeyFromKeystore(file, password)
	if err != nil {
		return nil, err
	}
	return &Account{
		NewKeySigner(key, chainID),
		reader,
		broadcaster,
		crypto.PubkeyToAddress(key.PublicKey),
	}, nil
}

func NewTrezorAccountGeneric(path string, address string, reader *reader.EthReader, broadcaster *broadcaster.Broadcaster, chainID int64) (*Account, error) {
	signer, err := trezoreum.NewTrezorSignerGeneric(path, address, chainID)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader,
		broadcaster,
		common.HexToAddress(address),
	}, nil
}

func NewLedgerAccountGeneric(path string, address string, reader *reader.EthReader, broadcaster *broadcaster.Broadcaster, chainID int64) (*Account, error) {
	signer, err := ledgereum.NewLedgerSignerGeneric(path, address, chainID)
	if err != nil {
		return nil, err
	}
	return &Account{
		signer,
		reader,
		broadcaster,
		common.HexToAddress(address),
	}, nil
}

func (self *Account) SetReader(r *reader.EthReader) {
	self.reader = r
}

func (self *Account) SetBroadcaster(b *broadcaster.Broadcaster) {
	self.broadcaster = b
}

func (self *Account) Address() string {
	return self.address.Hex()
}

func (self *Account) GetMinedNonce() (uint64, error) {
	return self.reader.GetMinedNonce(self.Address())
}

func (self *Account) GetPendingNonce() (uint64, error) {
	return self.reader.GetPendingNonce(self.Address())
}

func (self *Account) ListOfPendingNonces() ([]uint64, error) {
	minedNonce, err := self.GetMinedNonce()
	if err != nil {
		return []uint64{}, err
	}
	pendingNonce, err := self.GetPendingNonce()
	if err != nil {
		return []uint64{}, err
	}
	result := []uint64{}
	for i := minedNonce; i < pendingNonce; i++ {
		result = append(result, i)
	}
	return result, nil
}

func (self *Account) SendETHWithNonceAndPrice(nonce uint64, gasLimit uint64, priceGwei float64, ethAmount *big.Int, to string) (tx *types.Transaction, broadcasted bool, errors error) {
	tx = BuildExactSendETHTx(nonce, to, ethAmount, gasLimit, priceGwei)
	signedTx, err := self.signer.SignTx(tx)
	if err != nil {
		return tx, false, fmt.Errorf("couldn't sign the tx: %s", err)
	}
	_, broadcasted, errors = self.broadcaster.BroadcastTx(signedTx)
	return signedTx, broadcasted, errors
}

func (self *Account) ERC20Balance(tokenAddr string) (*big.Int, error) {
	return self.reader.ERC20Balance(tokenAddr, self.Address())
}

func (self *Account) ETHBalance() (*big.Int, error) {
	return self.reader.GetBalance(self.Address())
}

func (self *Account) SendAllETHWithPrice(priceGwei float64, to string) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	balance, err := self.reader.GetBalance(self.Address())
	if err != nil {
		return nil, false, fmt.Errorf("cannot get balance: %s", err)
	}
	amount := balance.Sub(balance, big.NewInt(0).Mul(big.NewInt(30000), FloatToBigInt(priceGwei, 9)))
	if amount.Cmp(big.NewInt(0)) != 1 {
		return nil, false, fmt.Errorf("not enough to do a tx with gas price: %f gwei", priceGwei)
	}
	return self.SendETHWithNonceAndPrice(nonce, 30000, priceGwei, amount, to)
}

func (self *Account) SendAllETH(to string) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get recommended gas price: %s", err)
	}
	balance, err := self.reader.GetBalance(self.Address())
	if err != nil {
		return nil, false, fmt.Errorf("cannot get balance: %s", err)
	}
	amount := balance.Sub(balance, big.NewInt(0).Mul(big.NewInt(30000), FloatToBigInt(priceGwei, 9)))
	if amount.Cmp(big.NewInt(0)) != 1 {
		return nil, false, fmt.Errorf("not enough to do a tx with gas price: %f gwei", priceGwei)
	}
	return self.SendETHWithNonceAndPrice(nonce, 30000, priceGwei, amount, to)
}

func (self *Account) SetERC20Allowance(tokenAddr string, spender string, tokenAmount float64) (tx *types.Transaction, broadcasted bool, errors error) {
	decimals, err := self.reader.ERC20Decimal(tokenAddr)
	if err != nil {
		return nil, false, fmt.Errorf("cannot get token decimal: %s", err)
	}
	amount := FloatToBigInt(tokenAmount, decimals)
	return self.CallContract(
		150000, 0, tokenAddr, "approve",
		HexToAddress(spender), amount)
}

func (self *Account) SendAllERC20(tokenAddr string, to string) (tx *types.Transaction, broadcasted bool, errors error) {
	balance, err := self.ERC20Balance(tokenAddr)
	if err != nil {
		return nil, false, fmt.Errorf("cannot get token balance: %s", err)
	}
	return self.CallERC20Contract(150000, 0, tokenAddr, "transfer", HexToAddress(to), balance)
}

func (self *Account) SendERC20(tokenAddr string, tokenAmount float64, to string) (tx *types.Transaction, broadcasted bool, errors error) {
	decimals, err := self.reader.ERC20Decimal(tokenAddr)
	if err != nil {
		return nil, false, fmt.Errorf("cannot get token decimal: %s", err)
	}
	amount := FloatToBigInt(tokenAmount, decimals)
	return self.CallERC20Contract(150000, 0, tokenAddr, "transfer", HexToAddress(to), amount)
}

func (self *Account) SendETH(ethAmount float64, to string) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get recommended gas price: %s", err)
	}
	amount := FloatToBigInt(ethAmount, 18)
	return self.SendETHWithNonceAndPrice(nonce, 30000, priceGwei, amount, to)
}

func (self *Account) SendETHToMultipleAddressesWithPrice(priceGwei float64, amounts []float64, addresses []string) (txs []*types.Transaction, broadcasteds []bool, errors []error) {
	if len(amounts) != len(addresses) {
		panic("amounts and addresses must have the same length")
		return
	}
	nonce, err := self.GetMinedNonce()
	if err != nil {
		panic(fmt.Errorf("cannot get nonce: %s", err))
		return
	}
	txs = []*types.Transaction{}
	broadcasteds = []bool{}
	errors = []error{}
	for i, addr := range addresses {
		amount := amounts[i]
		newNonce := nonce + uint64(i)
		tx, broadcasted, e := self.SendETHWithNonceAndPrice(newNonce, 30000, priceGwei, FloatToBigInt(amount, 18), addr)
		txs = append(txs, tx)
		broadcasteds = append(broadcasteds, broadcasted)
		errors = append(errors, e)
	}
	return txs, broadcasteds, errors
}

func (self *Account) SendETHToMultipleAddresses(amounts []float64, addresses []string) (txs []*types.Transaction, broadcasteds []bool, errors []error) {
	if len(amounts) != len(addresses) {
		panic("amounts and addresses must have the same length")
		return
	}
	nonce, err := self.GetMinedNonce()
	if err != nil {
		panic(fmt.Errorf("cannot get nonce: %s", err))
		return
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		panic(fmt.Errorf("cannot get recommended gas price: %s", err))
	}
	txs = []*types.Transaction{}
	broadcasteds = []bool{}
	errors = []error{}
	for i, addr := range addresses {
		amount := amounts[i]
		newNonce := nonce + uint64(i)
		tx, broadcasted, e := self.SendETHWithNonceAndPrice(newNonce, 30000, priceGwei, FloatToBigInt(amount, 18), addr)
		txs = append(txs, tx)
		broadcasteds = append(broadcasteds, broadcasted)
		errors = append(errors, e)
	}
	return txs, broadcasteds, errors
}

func (self *Account) CallERC20ContractWithPrice(
	priceGwei float64, extraGas uint64, value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	return self.CallERC20ContractWithNonceAndPrice(
		nonce, priceGwei, extraGas, value, caddr, function, params...)
}

func (self *Account) CallContractWithPrice(
	priceGwei float64, extraGas uint64, value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	return self.CallContractWithNonceAndPrice(
		nonce, priceGwei, extraGas, value, caddr, function, params...)
}

func (self *Account) CallERC20Contract(
	extraGas uint64,
	value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get recommended gas price: %s", err)
	}
	return self.CallERC20ContractWithNonceAndPrice(
		nonce, priceGwei, extraGas, value, caddr, function, params...)
}

func (self *Account) CallContract(
	extraGas uint64,
	value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get recommended gas price: %s", err)
	}
	return self.CallContractWithNonceAndPrice(
		nonce, priceGwei, extraGas, value, caddr, function, params...)
}

func (self *Account) PackERC20Data(function string, params ...interface{}) ([]byte, error) {
	abi := GetERC20ABI()
	return abi.Pack(function, params...)
}

func (self *Account) PackDataWithABI(a *abi.ABI, function string, params ...interface{}) ([]byte, error) {
	return a.Pack(function, params...)
}

func (self *Account) PackData(caddr string, function string, params ...interface{}) ([]byte, error) {
	abi, err := self.reader.GetABI(caddr)
	if err != nil {
		return []byte{}, fmt.Errorf("Cannot get ABI from scanner for %s", caddr)
	}
	return abi.Pack(function, params...)
}

func (self *Account) CallERC20ContractWithNonceAndPrice(
	nonce uint64, priceGwei float64, extraGas uint64,
	value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {
	if value < 0 {
		panic("value must be non-negative")
	}
	data, err := self.PackERC20Data(function, params...)
	if err != nil {
		return nil, false, fmt.Errorf("Cannot pack the params: %s", err)
	}
	gasLimit, err := self.reader.EstimateGas(
		self.Address(), caddr, priceGwei, value, data)
	if err != nil {
		return nil, false, fmt.Errorf("Cannot estimate gas: %s", err)
	}
	gasLimit += extraGas
	tx = BuildTx(nonce, caddr, value, gasLimit, priceGwei, data)
	signedTx, err := self.signer.SignTx(tx)
	if err != nil {
		return tx, false, fmt.Errorf("couldn't sign the tx: %s", err)
	}
	_, broadcasted, errors = self.broadcaster.BroadcastTx(signedTx)
	return signedTx, broadcasted, errors
}

func (self *Account) DeployContract(
	extraGas uint64, value float64, abiJson string, bytecode []byte,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, caddr common.Address, errors error) {
	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, common.Address{}, fmt.Errorf("cannot get nonce: %s", err)
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		return nil, false, common.Address{}, fmt.Errorf("cannot get recommended gas price: %s", err)
	}
	return self.DeployContractWithNonceAndPrice(
		nonce, priceGwei, extraGas, value, abiJson, bytecode,
		params...)
}

func (self *Account) DeployContractWithNonceAndPrice(
	nonce uint64, priceGwei float64, extraGas uint64,
	value float64, abiJson string, bytecode []byte,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, caddr common.Address, errors error) {
	a, err := abi.JSON(strings.NewReader(abiJson))
	if err != nil {
		return nil, false, common.Address{}, err
	}
	input, err := a.Pack("", params...)
	if err != nil {
		return nil, false, common.Address{}, err
	}
	fmt.Printf("Constructor abi encoding: %s\n", hexutil.Encode(input))
	data := append(bytecode, input...)

	gasLimit, err := self.reader.EstimateGas(
		self.Address(), "", priceGwei, value, data)
	if err != nil {
		return nil, false, common.Address{}, fmt.Errorf("Cannot estimate gas: %s", err)
	}
	gasLimit += extraGas

	amount := FloatToBigInt(value, 18)
	tx = BuildContractCreationTx(nonce, amount, gasLimit, priceGwei, data)
	signedTx, err := self.signer.SignTx(tx)
	if err != nil {
		return tx, false, common.Address{}, fmt.Errorf("couldn't sign the tx: %s", err)
	}
	_, broadcasted, errors = self.broadcaster.BroadcastTx(signedTx)
	caddr = crypto.CreateAddress(self.address, tx.Nonce())
	return signedTx, broadcasted, caddr, errors
}

func (self *Account) CallContractWithABI(
	a *abi.ABI, extraGas uint64,
	value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {

	nonce, err := self.GetMinedNonce()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get nonce: %s", err)
	}
	priceGwei, err := self.reader.RecommendedGasPrice()
	if err != nil {
		return nil, false, fmt.Errorf("cannot get recommended gas price: %s", err)
	}
	return self.CallContractWithABINonceAndPrice(
		a, nonce, priceGwei, extraGas, value, caddr, function, params...)
}

func (self *Account) CallContractWithABINonceAndPrice(
	a *abi.ABI, nonce uint64, priceGwei float64, extraGas uint64,
	value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {

	if value < 0 {
		panic("value must be non-negative")
	}
	data, err := self.PackDataWithABI(a, function, params...)
	if err != nil {
		return nil, false, fmt.Errorf("Cannot pack the params: %s", err)
	}
	gasLimit, err := self.reader.EstimateGas(
		self.Address(), caddr, priceGwei, value, data)
	if err != nil {
		return nil, false, fmt.Errorf("Cannot estimate gas: %s", err)
	}
	gasLimit += extraGas
	tx = BuildTx(nonce, caddr, value, gasLimit, priceGwei, data)
	return self.SignTxAndBroadcast(tx)
}

func (self *Account) CallContractWithNonceAndPrice(
	nonce uint64, priceGwei float64, extraGas uint64,
	value float64, caddr string, function string,
	params ...interface{}) (tx *types.Transaction, broadcasted bool, errors error) {
	if value < 0 {
		panic("value must be non-negative")
	}
	data, err := self.PackData(caddr, function, params...)
	if err != nil {
		return nil, false, fmt.Errorf("Cannot pack the params: %s", err)
	}
	gasLimit, err := self.reader.EstimateGas(
		self.Address(), caddr, priceGwei, value, data)
	if err != nil {
		return nil, false, fmt.Errorf("Cannot estimate gas: %s", err)
	}
	gasLimit += extraGas
	tx = BuildTx(nonce, caddr, value, gasLimit, priceGwei, data)
	return self.SignTxAndBroadcast(tx)
}

func (self *Account) SignTx(tx *types.Transaction) (*types.Transaction, error) {
	signedTx, err := self.signer.SignTx(tx)
	if err != nil {
		return tx, fmt.Errorf("Couldn't sign the tx: %s", err)
	}
	return signedTx, nil
}

func (self *Account) Broadcast(tx *types.Transaction) (*types.Transaction, bool, error) {
	_, broadcasted, err := self.broadcaster.BroadcastTx(tx)
	return tx, broadcasted, err
}

func (self *Account) SignTxAndBroadcast(tx *types.Transaction) (*types.Transaction, bool, error) {
	signedTx, err := self.SignTx(tx)
	if err != nil {
		return tx, false, err
	}
	return self.Broadcast(signedTx)
}
