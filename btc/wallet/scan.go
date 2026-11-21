package wallet

import (
	"context"
	"fmt"
	"sync"

	"github.com/tranvictor/jarvis/btc/esplora"
)

// GapLimit is how many consecutive never-used addresses end the scan of an
// HD branch (BIP-44). Funds sent to an address further out than this are
// not found, exactly as in every other BIP-44 wallet.
const GapLimit = 20

type Source interface {
	Address(ctx context.Context, addr string) (esplora.AddressStats, error)
	UTXOs(ctx context.Context, addr string) ([]esplora.UTXO, error)
}

type Coin struct {
	esplora.UTXO
	Derived
}

type Scan struct {
	Coins     []Coin
	Confirmed int64
	Pending   int64
	// NextReceive and NextChange are the first never-used index of each
	// branch. A keystore wallet has one address, so both stay 0 and it is
	// reused for change.
	NextReceive uint32
	NextChange  uint32
	// Addresses is every address the scan looked at that has history.
	Addresses []UsedAddress
}

type UsedAddress struct {
	Derived
	Stats esplora.AddressStats
}

func (s *Scan) Spendable() int64 {
	var n int64
	for _, c := range s.Coins {
		if c.Confirmed {
			n += c.Value
		}
	}
	return n
}

const scanBatch = 10

func (w *Wallet) Scan(ctx context.Context, src Source) (*Scan, error) {
	res := &Scan{}
	if w.single != nil {
		d, err := w.Derive(0, 0)
		if err != nil {
			return nil, err
		}
		if err := w.collect(ctx, src, d, res); err != nil {
			return nil, err
		}
		return res, nil
	}
	for branch := uint32(0); branch <= 1; branch++ {
		next, err := w.scanBranch(ctx, src, branch, res)
		if err != nil {
			return nil, err
		}
		if branch == 0 {
			res.NextReceive = next
		} else {
			res.NextChange = next
		}
	}
	return res, nil
}

type probe struct {
	d     Derived
	stats esplora.AddressStats
	err   error
}

func (w *Wallet) scanBranch(ctx context.Context, src Source, branch uint32, res *Scan) (uint32, error) {
	gap := 0
	var lastUsed int64 = -1
	for start := uint32(0); gap < GapLimit; start += scanBatch {
		batch := make([]probe, scanBatch)
		var wg sync.WaitGroup
		for i := range batch {
			d, err := w.Derive(branch, start+uint32(i))
			if err != nil {
				return 0, err
			}
			batch[i].d = d
			wg.Add(1)
			go func(p *probe) {
				defer wg.Done()
				p.stats, p.err = src.Address(ctx, p.d.Address)
			}(&batch[i])
		}
		wg.Wait()
		for _, p := range batch {
			if p.err != nil {
				return 0, fmt.Errorf("checking %s: %w", p.d.Address, p.err)
			}
			if p.stats.TxCount == 0 {
				gap++
				if gap >= GapLimit {
					break
				}
				continue
			}
			gap = 0
			lastUsed = int64(p.d.Index)
			if err := w.collectKnown(ctx, src, p.d, p.stats, res); err != nil {
				return 0, err
			}
		}
	}
	return uint32(lastUsed + 1), nil
}

func (w *Wallet) collect(ctx context.Context, src Source, d Derived, res *Scan) error {
	st, err := src.Address(ctx, d.Address)
	if err != nil {
		return fmt.Errorf("checking %s: %w", d.Address, err)
	}
	if st.TxCount == 0 {
		return nil
	}
	return w.collectKnown(ctx, src, d, st, res)
}

func (w *Wallet) collectKnown(ctx context.Context, src Source, d Derived, st esplora.AddressStats, res *Scan) error {
	res.Addresses = append(res.Addresses, UsedAddress{Derived: d, Stats: st})
	res.Confirmed += st.Confirmed
	res.Pending += st.Pending
	if st.Confirmed == 0 && st.Pending == 0 {
		return nil
	}
	us, err := src.UTXOs(ctx, d.Address)
	if err != nil {
		return fmt.Errorf("listing coins of %s: %w", d.Address, err)
	}
	for _, u := range us {
		res.Coins = append(res.Coins, Coin{UTXO: u, Derived: d})
	}
	return nil
}
