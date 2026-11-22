package broadcaster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/tranvictor/jarvis/common"
)

// TagMEVProtected marks an RPC node that keeps a transaction off the
// public mempool. Broadcast tries every node with this tag before any
// other node.
const TagMEVProtected = "mev-protected"

// ErrMEVFallback is returned alongside a successful broadcast when every
// mev-protected node rejected the transaction and it was then accepted by
// another node. errors.Is matches it through the wrapped per-node errors.
var ErrMEVFallback = errors.New("mev-protected nodes rejected the transaction; broadcast continued on the other nodes")

// Broadcaster sends a signed transaction to the nodes it could dial.
// Nodes tagged mev-protected are tried together first. Every other node
// is tried together only when each mev-protected node has rejected the
// transaction. With no mev-protected node available, every node is tried
// together.
type Broadcaster struct {
	nodes []rpcNode
}

type rpcNode struct {
	name   string
	url    string
	client *rpc.Client
	mev    bool
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
	hash := common.RawTxToHash(data)
	if len(b.nodes) == 0 {
		return hash, true, nil
	}

	var protected, normal []rpcNode
	for _, node := range b.nodes {
		if node.mev {
			protected = append(protected, node)
			continue
		}
		normal = append(normal, node)
	}

	if len(protected) == 0 {
		ok, err := b.broadcastAll(normal, data)
		return hash, ok, err
	}

	ok, err := b.broadcastAll(protected, data)
	if ok {
		return hash, true, nil
	}
	if len(normal) == 0 {
		return hash, false, err
	}
	ok, errNormal := b.broadcastAll(normal, data)
	if ok {
		if err == nil {
			err = errors.New("rejected")
		}
		return hash, true, fmt.Errorf("%w: %w", ErrMEVFallback, err)
	}
	return hash, false, errors.Join(err, errNormal)
}

// broadcastAll contacts every node in the set at once. ok is false only
// when every node returns an error.
func (b *Broadcaster) broadcastAll(nodes []rpcNode, data string) (bool, error) {
	if len(nodes) == 0 {
		return false, nil
	}
	timeout, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	tasks := make([]func() error, 0, len(nodes))
	for _, node := range nodes {
		node := node
		tasks = append(tasks, func() error {
			err := b.broadcast(timeout, node.client, data)
			if err != nil {
				return fmt.Errorf("node %q at %s: %w", node.name, node.url, err)
			}
			return nil
		})
	}
	err, numErrs := common.RunParallel(tasks...)
	if numErrs == len(nodes) {
		return false, err
	}
	return true, nil
}

func HasMEVTag(tags []string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), TagMEVProtected) {
			return true
		}
	}
	return false
}

func NewGenericBroadcaster(nodes map[string]string, tags map[string][]string) *Broadcaster {
	out := make([]rpcNode, 0, len(nodes))
	for name, rawURL := range nodes {
		url := common.CanonicalRPCURL(rawURL)
		client, err := rpc.Dial(url)
		if err != nil {
			continue
		}
		out = append(out, rpcNode{
			name:   name,
			url:    url,
			client: client,
			mev:    HasMEVTag(tags[name]),
		})
	}
	return &Broadcaster{nodes: out}
}
