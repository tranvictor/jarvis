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

// Broadcaster sends a signed tx to all managed nodes in parallel and
// returns success as soon as at least one node accepts it.
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

func (b *Broadcaster) broadcastSync(
	ctx context.Context,
	client *rpc.Client, data string,
) (*types.Receipt, error) {
	var receipt *types.Receipt
	err := client.CallContext(ctx, &receipt, "eth_sendRawTransactionSync", data)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func (b *Broadcaster) BroadcastTx(tx *types.Transaction) (string, bool, error) {
	data, err := tx.MarshalBinary()
	if err != nil {
		return "", false, fmt.Errorf("tx is not valid, couldn't use rlp to encode it: %w", err)
	}
	return b.Broadcast(hexutil.Encode(data))
}

func (b *Broadcaster) BroadcastTxSync(tx *types.Transaction) (*types.Receipt, error) {
	data, err := tx.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("tx is not valid, couldn't use rlp to encode it: %w", err)
	}
	receipt, success, err := b.BroadcastSync(hexutil.Encode(data))
	if !success {
		return nil, err
	}
	return receipt, nil
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

func (b *Broadcaster) BroadcastSync(data string) (*types.Receipt, bool, error) {
	parallelTasks := []func() error{}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	// Thread-safe storage for receipts
	var mu sync.Mutex
	var successfulReceipt *types.Receipt
	var hasSuccess bool

	for id := range b.clients {
		cli := b.clients[id]
		parallelTasks = append(parallelTasks, func() error {
			receipt, err := b.broadcastSync(timeout, cli, data)
			if err == nil && receipt != nil {
				mu.Lock()
				if !hasSuccess {
					successfulReceipt = receipt
					hasSuccess = true
				}
				mu.Unlock()
			}
			return err
		})
	}
	err, numErrs := common.RunParallel(parallelTasks...)
	if numErrs == len(b.clients) {
		return nil, false, err
	}

	if hasSuccess {
		return successfulReceipt, true, nil
	}
	return nil, false, fmt.Errorf("no successful broadcast found")
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
