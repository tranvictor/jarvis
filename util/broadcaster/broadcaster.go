package broadcaster

import (
	"context"
	"fmt"
	"log"
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

func (b *Broadcaster) GetNodes() map[string]*rpc.Client {
	return b.clients
}

func (b *Broadcaster) broadcast(
	ctx context.Context,
	client *rpc.Client, data string,
) error {
	return client.CallContext(ctx, nil, "eth_sendRawTransaction", data)
}

func (b *Broadcaster) BroadcastTx(tx *types.Transaction) (string, bool, error) {
	data, err := tx.MarshalBinary()
	if err != nil {
		return "", false, fmt.Errorf("tx is not valid, couldn't use rlp to encode it: %w", err)
	}
	return b.Broadcast(hexutil.Encode(data))
}

// data must be hex encoded of the signed tx
func (b *Broadcaster) Broadcast(data string) (string, bool, error) {
	parallelTasks := []func() error{}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for id := range b.clients {
		cli := b.clients[id]
		parallelTasks = append(parallelTasks, func() error {
			return b.broadcast(timeout, cli, data)
		})
	}
	err, numErrs := common.RunParallel(parallelTasks...)
	if numErrs == len(b.clients) {
		return common.RawTxToHash(data), false, err
	}

	return common.RawTxToHash(data), true, nil
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
