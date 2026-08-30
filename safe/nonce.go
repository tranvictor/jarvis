package safe

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// NextNonce returns the SafeTx nonce to use for a brand-new proposal.
// It combines the on-chain nonce with the collector's pending queue so
// multiple in-flight proposals don't collide.
//
// Without a collector there is no source of truth for the pending queue,
// so the on-chain nonce is returned and the caller can override it
// (for example with --safe-nonce).
func NextNonce(s *SafeContract, c SignatureCollector) (uint64, error) {
	onchain, err := s.Nonce()
	if err != nil {
		return 0, fmt.Errorf("reading on-chain nonce: %w", err)
	}
	if c == nil {
		return onchain, nil
	}
	return nextFreeNonce(onchain, func(nonce uint64) (*PendingTx, error) {
		return c.FindByNonce(common.HexToAddress(s.Address), nonce)
	})
}

// nextFreeNonce walks forward from start until find reports a free slot
// (nil pending). The collector is authoritative for "is there an
// in-flight proposal at nonce N?".
func nextFreeNonce(start uint64, find func(nonce uint64) (*PendingTx, error)) (uint64, error) {
	next := start
	for i := uint64(0); i < 64; i++ {
		pending, err := find(next)
		if err != nil {
			// On the first iteration treat lookup errors as fatal; otherwise
			// assume we've walked past the queue.
			if i == 0 {
				return 0, fmt.Errorf("checking pending queue at nonce %d: %w", next, err)
			}
			break
		}
		if pending == nil {
			return next, nil
		}
		next++
	}
	return next, nil
}
