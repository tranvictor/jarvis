package util

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/tranvictor/jarvis/accounts"
	. "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/txanalyzer"
	. "github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/account"
	. "github.com/tranvictor/jarvis/util/broadcaster"
	. "github.com/tranvictor/jarvis/util/reader"
)

var GAS_INFO_TTL = 60 * time.Second

type GasInfo struct {
	GasPrice         float64
	BaseGasPrice     *big.Int
	MaxPriorityPrice float64
	FeePerGas        float64
	Timestamp        time.Time
}

// ContextManager manages
//  1. multiple wallets and their informations in its
//     life time. It basically gives next nonce to do transaction for specific
//     wallet and specific network.
//     It queries the node to check the nonce in lazy maner, it also takes mining
//     txs into account.
//  2. multiple networks gas price. The gas price will be queried lazily prior to txs
//     and will be stored as cache for a while
//  3. txs in the context manager's life time
type ContextManager struct {
	lock sync.RWMutex

	// readers stores all reader instances for all networks that ever interacts
	// with accounts manager. ChainID of the network is used as the key.
	readers      map[uint64]*EthReader
	broadcasters map[uint64]*Broadcaster
	analyzers    map[uint64]*TxAnalyzer

	accounts map[common.Address]*account.Account
	// nonces map between (address, network) => last signed nonce (not mined nonces)
	pendingNonces map[common.Address]map[uint64]*big.Int
	// txs map between (address, network, nonce) => tx
	txs map[common.Address]map[uint64]map[uint64]*types.Transaction

	// gasPrices map between network => gasinfo
	gasSettings map[uint64]*GasInfo
}

func NewContextManager() *ContextManager {
	return &ContextManager{
		sync.RWMutex{},
		map[uint64]*EthReader{},
		map[uint64]*Broadcaster{},
		map[uint64]*TxAnalyzer{},
		map[common.Address]*account.Account{},
		map[common.Address]map[uint64]*big.Int{},
		map[common.Address]map[uint64]map[uint64]*types.Transaction{},
		map[uint64]*GasInfo{},
	}
}

func (cm *ContextManager) setAccount(acc *account.Account) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.accounts[acc.Address()] = acc
}

func (cm *ContextManager) UnlockAccount(addr common.Address) (*account.Account, error) {
	accDesc, err := accounts.GetAccount(addr.Hex())
	if err != nil {
		return nil, fmt.Errorf(
			"You don't control wallet %s yet. You might want to add it to jarvis.\n",
		)
	}
	acc, err := accounts.UnlockAccount(accDesc)
	if err != nil {
		return nil, fmt.Errorf("Unlocking wallet failed: %s.\n", err)
	}
	cm.setAccount(acc)
	return acc, nil
}

func (cm *ContextManager) Account(wallet common.Address) *account.Account {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return cm.accounts[wallet]
}

func (cm *ContextManager) setPendingNonce(wallet common.Address, network Network, nonce uint64) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	walletNonces := cm.pendingNonces[wallet]
	if walletNonces == nil {
		walletNonces = map[uint64]*big.Int{}
		cm.pendingNonces[wallet] = walletNonces
	}
	walletNonces[network.GetChainID()] = big.NewInt(int64(nonce))
}

func (cm *ContextManager) setTx(wallet common.Address, network Network, tx *types.Transaction) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	if cm.txs[wallet] == nil {
		cm.txs[wallet] = map[uint64]map[uint64]*types.Transaction{}
	}

	if cm.txs[wallet][network.GetChainID()] == nil {
		cm.txs[wallet][network.GetChainID()] = map[uint64]*types.Transaction{}
	}

	cm.txs[wallet][network.GetChainID()][uint64(tx.Nonce())] = tx
}

func (cm *ContextManager) getBroadcaster(network Network) *Broadcaster {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return cm.broadcasters[network.GetChainID()]
}

func (cm *ContextManager) Broadcaster(network Network) *Broadcaster {
	broadcaster := cm.getBroadcaster(network)
	if broadcaster == nil {
		err := cm.initNetwork(network)
		if err != nil {
			panic(
				fmt.Errorf(
					"couldn't init reader and broadcaster for network: %s, err: %s",
					network,
					err,
				),
			)
		}
		return cm.getBroadcaster(network)
	}
	return broadcaster
}

func (cm *ContextManager) getReader(network Network) *EthReader {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return cm.readers[network.GetChainID()]
}

func (cm *ContextManager) Reader(network Network) *EthReader {
	reader := cm.getReader(network)
	if reader == nil {
		err := cm.initNetwork(network)
		if err != nil {
			panic(
				fmt.Errorf(
					"couldn't init reader and broadcaster for network: %s, err: %s",
					network,
					err,
				),
			)
		}
		return cm.getReader(network)
	}
	return reader
}

func (cm *ContextManager) getAnalyzer(network Network) *TxAnalyzer {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return cm.analyzers[network.GetChainID()]
}

func (cm *ContextManager) Analyzer(network Network) *TxAnalyzer {
	analyzer := cm.getAnalyzer(network)
	if analyzer == nil {
		err := cm.initNetwork(network)
		if err != nil {
			panic(
				fmt.Errorf(
					"couldn't init reader and broadcaster for network: %s, err: %s",
					network,
					err,
				),
			)
		}
		return cm.getAnalyzer(network)
	}
	return analyzer
}

func (cm *ContextManager) initNetwork(network Network) (err error) {
	cm.lock.Lock()
	defer cm.lock.Unlock()

	reader, found := cm.readers[network.GetChainID()]
	if !found {
		reader, err = util.EthReader(network)
		if err != nil {
			return err
		}
	}
	cm.readers[network.GetChainID()] = reader

	analyzer, found := cm.analyzers[network.GetChainID()]
	if !found {
		analyzer = txanalyzer.NewGenericAnalyzer(reader, network)
		if err != nil {
			return err
		}
	}
	cm.analyzers[network.GetChainID()] = analyzer

	broadcaster, found := cm.broadcasters[network.GetChainID()]
	if !found {
		broadcaster, err = util.EthBroadcaster(network)
		if err != nil {
			return err
		}
	}
	cm.broadcasters[network.GetChainID()] = broadcaster
	return nil
}

func (cm *ContextManager) PendingNonce(wallet common.Address, network Network) *big.Int {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	walletPendingNonces := cm.pendingNonces[wallet]
	if walletPendingNonces == nil {
		return nil
	}
	return walletPendingNonces[network.GetChainID()]
}

//  1. get remote pending nonce
//  2. get local pending nonce
//  2. get mined nonce
//  3. if mined nonce == remote == local, all good, lets return the mined nonce
//  4. since mined nonce is always <= remote nonce, if mined nonce > local nonce,
//     this session doesn't catch up with mined txs (in case there are txs  that
//     were from other apps and they were mined), return max(mined none, remote nonce)
//     and set local nonce to max(mined none, remote nonce)
//  5. if not, means mined nonce is smaller than both remote and local pending nonce
//     5.1 if remote == local: means all pending txs are from this session, we return
//     local nonce
//     5.2 if remote > local: means there is pending txs from another app, we return
//     remote nonce in order not to mess up with the other txs, but give a warning
//     5.3 if local > remote: means txs from this session are not broadcasted to the
//     the notes, return local nonce and give warnings
func (cm *ContextManager) Nonce(wallet common.Address, network Network) (*big.Int, error) {
	reader := cm.Reader(network)
	minedNonce, err := reader.GetMinedNonce(wallet.Hex())
	if err != nil {
		return nil, fmt.Errorf("couldn't get mined nonce in context manager: %s", err)
	}
	remotePendingNonce, err := reader.GetPendingNonce(wallet.Hex())
	if err != nil {
		return nil, fmt.Errorf("couldn't get remote pending nonce in context manager: %s", err)
	}
	var localPendingNonce uint64
	localPendingNonceBig := cm.PendingNonce(wallet, network)
	if localPendingNonceBig == nil {
		cm.setPendingNonce(wallet, network, remotePendingNonce)
		localPendingNonce = remotePendingNonce
	} else {
		localPendingNonce = localPendingNonceBig.Uint64()
	}

	hasPendingTxsOnNodes := minedNonce < remotePendingNonce
	if !hasPendingTxsOnNodes {
		if minedNonce > remotePendingNonce {
			return nil, fmt.Errorf(
				"mined nonce is higher than pending nonce, this is abnormal data from nodes, retry again later",
			)
		}
		// in this case, minedNonce is supposed to == remotePendingNonce
		if localPendingNonce <= minedNonce {
			// in this case, minedNonce is more up to date, update localPendingNonce
			// and return minedNonce
			cm.setPendingNonce(wallet, network, minedNonce)
			return big.NewInt(int64(minedNonce)), nil
		} else {
			// in this case, local is more up to date, return pending nonce
			return big.NewInt(int64(localPendingNonce)), nil
		}
	}

	if localPendingNonce <= minedNonce {
		// localPendingNonce <= minedNonce < remotePendingNonce
		// in this case, there are pending txs on nodes and they are
		// from other apps
		// TODO: put warnings
		// we don't have to update local pending nonce here since
		// it will be updated if the new tx is broadcasted with context manager
		return big.NewInt(int64(remotePendingNonce)), nil
	} else if localPendingNonce <= remotePendingNonce {
		// minedNonce < localPendingNonce <= remotePendingNonce
		// similar to the previous case, however, there are pending txs came from
		// jarvis as well. No need special treatments
		return big.NewInt(int64(remotePendingNonce)), nil
	}
	// minedNonce < remotePendingNonce < localPendingNonce
	// in this case, local has more pending txs, this is the case when
	// the node doesn't have full pending txs as local, something is
	// wrong with the local txs.
	// TODO: give warnings and check pending txs, see if they are not found and update
	// local pending nonce respectively and retry not found txs, need to figure out
	// a mechanism to stop trying as well.
	// For now, we will just go ahead with localPendingNonce
	return big.NewInt(int64(localPendingNonce)), nil
}

func (cm *ContextManager) getGasSettingInfo(network Network) *GasInfo {
	cm.lock.RLock()
	defer cm.lock.RUnlock()
	return cm.gasSettings[network.GetChainID()]
}

func (cm *ContextManager) setGasInfo(network Network, info *GasInfo) {
	cm.lock.Lock()
	defer cm.lock.Unlock()
	cm.gasSettings[network.GetChainID()] = info
}

// implement a cache mechanism to be more efficient
func (cm *ContextManager) GasSetting(network Network) (*GasInfo, error) {
	gasInfo := cm.getGasSettingInfo(network)
	if gasInfo == nil || time.Since(gasInfo.Timestamp) >= GAS_INFO_TTL {
		// gasInfo is not initiated or outdated
		reader := cm.Reader(network)
		gasPrice, gasTipCapGwei, err := reader.SuggestedGasSettings()
		if err != nil {
			return nil, fmt.Errorf("Couldn't get gas settings in context manager: %s", err)
		}

		info := GasInfo{
			GasPrice:         gasPrice,
			BaseGasPrice:     nil,
			MaxPriorityPrice: gasTipCapGwei,
			FeePerGas:        gasPrice,
			Timestamp:        time.Now(),
		}
		cm.setGasInfo(network, &info)
		return &info, nil
	}
	return cm.getGasSettingInfo(network), nil
}

func (cm *ContextManager) BroadcastRawTx(
	data string,
) (hash string, successful bool, allErrors error) {
	rawTxBytes, err := hex.DecodeString(data)
	if err != nil {
		return "", false, fmt.Errorf(
			"couldn't decode hex string. txdata should be in hex format WITHOUT 0x prefix",
		)
	}

	tx := new(types.Transaction)
	rlp.DecodeBytes(rawTxBytes, &tx)
	return cm.BroadcastTx(tx)
}

func (cm *ContextManager) BuildTx(
	txType uint8,
	from, to common.Address,
	nonce *big.Int,
	value *big.Int,
	gasLimit uint64,
	gasPrice float64,
	tipCapGwei float64,
	data []byte,
	network Network,
) (tx *types.Transaction, err error) {
	if gasLimit == 0 {
		gasLimit, err = cm.Reader(network).EstimateExactGas(
			from.Hex(), to.Hex(),
			gasPrice,
			value,
			data,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"Couldn't estimate gas. The tx is meant to revert or network error. Detail: %s",
				err,
			)
		}
	}

	if nonce == nil {
		nonce, err = cm.Nonce(from, network)
		if err != nil {
			return nil, fmt.Errorf("Couldn't get nonce of the wallet from any nodes: %s", err)
		}
	}

	if gasPrice == 0 {
		gasInfo, err := cm.GasSetting(network)
		if err != nil {
			return nil, fmt.Errorf("Couldn't get gas price info from any nodes: %s", err)
		}
		gasPrice = gasInfo.GasPrice
		tipCapGwei = gasInfo.MaxPriorityPrice
	}

	return BuildExactTx(
		txType,
		nonce.Uint64(),
		to.Hex(),
		value,
		gasLimit+config.ExtraGasLimit,
		gasPrice+config.ExtraGasPrice,
		tipCapGwei,
		data,
		network.GetChainID(),
	), nil
}

func (cm *ContextManager) SignTx(
	wallet common.Address,
	tx *types.Transaction,
	network Network,
) (signedTx *types.Transaction, err error) {
	acc := cm.Account(wallet)
	if acc == nil {
		acc, err = cm.UnlockAccount(wallet)
		if err != nil {
			return nil, fmt.Errorf("the wallet to sign txs is not registered in context manager")
		}
	}
	return acc.SignTx(tx, big.NewInt(int64(network.GetChainID())))
}

func (cm *ContextManager) SignTxAndBroadcast(
	wallet common.Address,
	tx *types.Transaction,
	network Network,
) (signedTx *types.Transaction, successful bool, allErrors error) {
	tx, err := cm.SignTx(wallet, tx, network)
	if err != nil {
		return tx, false, err
	}
	_, broadcasted, allErrors := cm.BroadcastTx(tx)
	return tx, broadcasted, allErrors
}

func (cm *ContextManager) registerBroadcastedTx(tx *types.Transaction, network Network) error {
	wallet, err := GetSignerAddressFromTx(tx, big.NewInt(int64(network.GetChainID())))
	if err != nil {
		return fmt.Errorf("couldn't derive sender from the tx data in context manager: %s", err)
	}
	// update nonce
	cm.setPendingNonce(wallet, network, tx.Nonce()+1)
	// update txs
	cm.setTx(wallet, network, tx)
	return nil
}

func (cm *ContextManager) BroadcastTx(
	tx *types.Transaction,
) (hash string, broadcasted bool, allErrors error) {
	network, err := GetNetworkByID(tx.ChainId().Uint64())
	// TODO: handle chainId 0 for old txs
	if err != nil {
		return "", false, fmt.Errorf("The tx is encoded with unsupported ChainID: %s", err)
	}
	hash, broadcasted, allErrors = cm.Broadcaster(network).BroadcastTx(tx)
	if broadcasted {
		cm.registerBroadcastedTx(tx, network)
	}
	return hash, broadcasted, allErrors
}
