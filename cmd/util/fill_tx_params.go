package util

import (
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/ui"
	jarvisutil "github.com/tranvictor/jarvis/util"
)

// FillSigningTxParams sets GasPrice, Nonce, TxType, TipGas, and Broadcaster
// on tc from config flags and the injected Reader. network is the chain
// those RPC calls should target (batch approve can differ from
// config.Network()).
//
// If tc.Broadcaster is already set it is left alone so tests and
// cross-network batch construction can inject their own.
func FillSigningTxParams(u ui.UI, tc *TxContext, network jarvisnetworks.Network) error {
	reader := tc.Reader

	var (
		price    float64
		nonce    uint64
		txType   uint8
		priceErr error
		nonceErr error
		typeErr  error
	)

	needPrice := config.GasPrice == 0
	needNonce := config.Nonce == 0

	var wg sync.WaitGroup
	if needPrice {
		wg.Add(1)
		go func() {
			defer wg.Done()
			price, priceErr = reader.RecommendedGasPrice()
		}()
	}
	if needNonce {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nonce, nonceErr = reader.GetMinedNonce(tc.From)
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		txType, typeErr = ValidTxType(reader, network)
	}()
	wg.Wait()

	if needPrice {
		if priceErr != nil {
			if u != nil {
				showNodeErrorGuidance(u, network)
			}
			return fmt.Errorf("getting recommended gas price failed: %w", priceErr)
		}
		tc.GasPrice = price
	} else {
		tc.GasPrice = config.GasPrice
	}

	if needNonce {
		if nonceErr != nil {
			if u != nil {
				showNodeErrorGuidance(u, network)
			}
			return fmt.Errorf("getting nonce failed: %w", nonceErr)
		}
		tc.Nonce = nonce
	} else {
		tc.Nonce = config.Nonce
	}

	if typeErr != nil {
		if u != nil {
			showNodeErrorGuidance(u, network)
		}
		return fmt.Errorf("couldn't determine proper tx type: %w", typeErr)
	}
	tc.TxType = txType

	if tc.TxType == types.LegacyTxType {
		if config.TipGas > 0 && u != nil {
			u.Warn("Legacy tx: ignoring --tipgas (EIP-1559 tips apply only to type-2 txs).")
		}
	} else if tc.TxType == types.DynamicFeeTxType {
		if config.TipGas == 0 {
			tip, err := reader.GetSuggestedGasTipCap()
			if err != nil {
				if u != nil {
					showNodeErrorGuidance(u, network)
				}
				return fmt.Errorf("couldn't estimate recommended gas price: %w", err)
			}
			tc.TipGas = tip
		} else {
			tc.TipGas = config.TipGas
		}
	}

	if tc.Broadcaster == nil {
		bc, err := jarvisutil.EthBroadcaster(network)
		if err != nil {
			if u != nil {
				showNodeErrorGuidance(u, network)
			}
			return fmt.Errorf("couldn't connect to broadcaster: %w", err)
		}
		tc.Broadcaster = bc
	}
	return nil
}
