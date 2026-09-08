package reader

// EIP-1559 pricing bumps applied after the node oracles.
const (
	// gasPriceHeadroom is multiplied onto eth_gasPrice to form maxFeePerGas
	// so the next blocks can raise base fee without the tx becoming
	// underpriced.
	gasPriceHeadroom = 1.5

	// When the oracles collapse to ~0, still offer miners something.
	// L1-like chains (base fee ≥ 1 gwei) use 1 gwei — the usual wallet
	// default. Cheap L2s use 0.01 gwei so a quiet Arbitrum tx is not
	// charged a full Ethereum tip.
	l1TipFloorGwei      = 1.0
	l2TipFloorGwei      = 0.01
	l1BaseFeeCutoffGwei = 1.0
)

// EffectiveGasTipGwei picks the type-2 priority fee (gwei).
//
// nodeTip is the node's eth_maxPriorityFeePerGas (already * tipBump).
// maxFee is Jarvis's maxFeePerGas (eth_gasPrice * gasPriceHeadroom).
// baseFee is the latest block base fee.
//
// eth_maxPriorityFeePerGas is often ~0 on Ethereum even when maxFee looks
// generous: miners are paid min(maxFee − baseFee, tip), so a 0 tip sits
// or is dropped. We take the better of the node tip, the tip implied by
// eth_gasPrice − baseFee, and a small floor, then clamp to maxFee.
func EffectiveGasTipGwei(nodeTip, maxFee, baseFee float64) float64 {
	if nodeTip < 0 {
		nodeTip = 0
	}
	if maxFee < 0 {
		maxFee = 0
	}
	if baseFee < 0 {
		baseFee = 0
	}

	gasPrice := maxFee / gasPriceHeadroom
	marketTip := gasPrice - baseFee
	if marketTip < 0 {
		marketTip = 0
	}

	floor := l2TipFloorGwei
	if baseFee >= l1BaseFeeCutoffGwei {
		floor = l1TipFloorGwei
	}

	tip := nodeTip
	if marketTip > tip {
		tip = marketTip
	}
	if floor > tip {
		tip = floor
	}
	if maxFee > 0 && tip > maxFee {
		tip = maxFee
	}
	return tip
}
