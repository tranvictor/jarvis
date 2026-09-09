package broadcaster

import (
	"context"
	"fmt"
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
	urls    map[string]string // node name -> RPC URL (for error messages)
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
		nodeName := id
		cli := b.clients[id]
		rpcURL := b.urls[nodeName]
		parallelTasks = append(parallelTasks, func() error {
			err := b.broadcast(timeout, cli, data)
			if err != nil {
				return fmt.Errorf("node %q at %s: %w", nodeName, rpcURL, err)
			}
			return nil
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
	urls := make(map[string]string, len(nodes))
	for name, c := range nodes {
		c = common.CanonicalRPCURL(c)
		urls[name] = c
		client, err := rpc.Dial(c)
		if err != nil {
			continue
		}
		clients[name] = client
	}
	return &Broadcaster{
		clients: clients,
		urls:    urls,
	}
}
