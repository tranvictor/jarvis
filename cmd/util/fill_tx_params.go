package util

import (
	"fmt"

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
	var err error

	if config.GasPrice == 0 {
		tc.GasPrice, err = reader.RecommendedGasPrice()
		if err != nil {
			if u != nil {
				showNodeErrorGuidance(u, network)
			}
			return fmt.Errorf("getting recommended gas price failed: %w", err)
		}
	} else {
		tc.GasPrice = config.GasPrice
	}

	if config.Nonce == 0 {
		tc.Nonce, err = reader.GetMinedNonce(tc.From)
		if err != nil {
			if u != nil {
				showNodeErrorGuidance(u, network)
			}
			return fmt.Errorf("getting nonce failed: %w", err)
		}
	} else {
		tc.Nonce = config.Nonce
	}

	tc.TxType, err = ValidTxType(reader, network)
	if err != nil {
		if u != nil {
			showNodeErrorGuidance(u, network)
		}
		return fmt.Errorf("couldn't determine proper tx type: %w", err)
	}

	if tc.TxType == types.LegacyTxType {
		if config.TipGas > 0 && u != nil {
			u.Warn("We are doing legacy tx hence we ignore tip gas parameter.")
		}
	} else if tc.TxType == types.DynamicFeeTxType {
		if config.TipGas == 0 {
			tc.TipGas, err = reader.GetSuggestedGasTipCap()
			if err != nil {
				if u != nil {
					showNodeErrorGuidance(u, network)
				}
				return fmt.Errorf("couldn't estimate recommended gas price: %w", err)
			}
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
