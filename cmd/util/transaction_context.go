package util

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/spf13/cobra"

	jtypes "github.com/tranvictor/jarvis/accounts/types"
	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/config"
	jarvisnetworks "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/reader"
)

// TxBroadcaster is the minimal interface consumed by the cmd layer for
// submitting signed transactions. Tests can supply a mock implementation.
type TxBroadcaster interface {
	BroadcastTx(tx *types.Transaction) (string, bool, error)
}

// TxContext holds all values derived during pre-processing: resolved addresses,
// auto-fetched gas parameters, parsed prefill strings, and optional TxInfo from
// a scanned tx hash. Commands retrieve it via TxContextFrom instead of reading
// mutated config.* globals.
type TxContext struct {
	FromAcc       jtypes.AccDesc
	From          string             // resolved hex address of the signer
	To            string             // resolved hex address; empty for contract creation
	Value         *big.Int           // parsed from config.RawValue
	GasPrice      float64            // gwei; auto-fetched when config.GasPrice == 0
	TipGas        float64            // gwei; auto-fetched for dynamic-fee txs
	Nonce         uint64             // auto-fetched when config.Nonce == 0
	TxType        uint8
	PrefillMode   bool
	PrefillParams []string
	TxInfo      *jarviscommon.TxInfo // non-nil when args[0] was a tx hash
	Reader      reader.Reader       // injected by preprocess; nil in tests that don't need network I/O
	Broadcaster TxBroadcaster       // injected by CommonTxPreprocess; nil for read-only commands
}

type txContextKey struct{}

// WithTxContext attaches tc to ctx and returns the new context.
func WithTxContext(ctx context.Context, tc TxContext) context.Context {
	return context.WithValue(ctx, txContextKey{}, tc)
}

// TxContextFrom retrieves the TxContext attached to cmd by a pre-run hook.
// The bool is false when no context has been attached (a zero-value TxContext is returned).
func TxContextFrom(cmd *cobra.Command) (TxContext, bool) {
	tc, ok := cmd.Context().Value(txContextKey{}).(TxContext)
	return tc, ok
}

// ValidTxType returns the appropriate transaction type for the current network,
// respecting config.ForceLegacy.
func ValidTxType(r reader.Reader, network jarvisnetworks.Network) (uint8, error) {
	if config.ForceLegacy {
		return types.LegacyTxType, nil
	}

	isDynamicFeeAvailable, err := r.CheckDynamicFeeTxAvailable()
	if err != nil {
		return 0, fmt.Errorf("couldn't check if the chain support dynamic fee: %w", err)
	}

	if !isDynamicFeeAvailable {
		return types.LegacyTxType, nil
	}

	return types.DynamicFeeTxType, nil
}
