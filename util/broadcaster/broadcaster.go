package broadcaster

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/tranvictor/jarvis/common"
)

// Broadcaster takes a signed tx and try to broadcast it to all
// nodes that it manages as fast as possible. It returns a map of
// failures and a bool indicating that the tx is broadcasted to
// at least 1 node
type Broadcaster struct {
	clients map[string]*rpc.Client
}

func (self *Broadcaster) GetNodes() map[string]*rpc.Client {
	return self.clients
}

func (self *Broadcaster) broadcast(
	ctx context.Context,
	id string, client *rpc.Client, data string,
	wg *sync.WaitGroup, failures *sync.Map,
) {
	defer wg.Done()
	err := client.CallContext(ctx, nil, "eth_sendRawTransaction", data)
	if err != nil {
		failures.Store(id, err)
	}
}

func (self *Broadcaster) BroadcastTx(tx *types.Transaction) (string, bool, error) {
	data, err := tx.MarshalBinary()
	if err != nil {
		return "", false, makeError(map[string]error{
			"tx": fmt.Errorf("Tx is not valid, couldn't use rlp to encode it"),
		})
	}
	return self.Broadcast(hexutil.Encode(data))
}

// data must be hex encoded of the signed tx
func (self *Broadcaster) Broadcast(data string) (string, bool, error) {
	failures := sync.Map{}
	wg := sync.WaitGroup{}
	for id := range self.clients {
		wg.Add(1)
		timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		cli := self.clients[id]
		go self.broadcast(timeout, id, cli, data, &wg, &failures)
		defer cancel()
	}
	wg.Wait()
	result := map[string]error{}
	failures.Range(func(key, value interface{}) bool {
		k, ok := key.(string)
		if !ok {
			log.Printf("Broadcast: key (%v) cannot be asserted to string", key)
			return true
		}
		err, ok := value.(error)
		if !ok {
			log.Printf("Broadcast: value (%v) cannot be asserted to error", value)
			return true
		}
		result[k] = err
		return true
	})
	return common.RawTxToHash(
			data,
		), len(result) != len(self.clients) &&
			len(self.clients) > 0, makeError(
			result,
		)
}

func NewGenericBroadcaster(nodes map[string]string) *Broadcaster {
	clients := map[string]*rpc.Client{}
	for name, c := range nodes {
		client, err := rpc.Dial(c)
		if err != nil {
			log.Printf("Couldn't connect to: %s - %v", c, err)
		} else {
			clients[name] = client
		}
	}
	return &Broadcaster{
		clients: clients,
	}
}

func NewBSCBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"binance":  "https://bsc-dataseed.binance.org",
		"defibit":  "https://bsc-dataseed1.defibit.io",
		"ninicoin": "https://bsc-dataseed1.ninicoin.io",
	}
	return NewGenericBroadcaster(nodes)
}

func NewBSCTestnetBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
		"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
		"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
	}
	return NewGenericBroadcaster(nodes)
}

func NewKovanBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewGenericBroadcaster(nodes)
}

func NewRinkebyBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewGenericBroadcaster(nodes)
}

func NewRopstenBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"ropsten-infura": "https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewGenericBroadcaster(nodes)
}

func NewTomoBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"mainnet-tomo": "https://rpc.tomochain.com",
	}
	return NewGenericBroadcaster(nodes)
}

func NewBroadcaster() *Broadcaster {
	nodes := map[string]string{
		"mainnet-alchemy": "https://eth-mainnet.alchemyapi.io/jsonrpc/YP5f6eM2wC9c2nwJfB0DC1LObdSY7Qfv",
		"mainnet-infura":  "https://mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewGenericBroadcaster(nodes)
}
