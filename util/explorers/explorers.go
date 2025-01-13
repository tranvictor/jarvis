package explorers

type BlockExplorer interface {
	RecommendedGasPrice() (float64, error)
	GetABIString(address string) (string, error)
}

func NewEtherscanV2() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api.etherscan.io/v2",
		"UBB257TI824FC7HUSPT66KZUMGBPRN3IWV",
	)
}

func NewMainnetEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewRopstenEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewRinkebyEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewKovanEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewBscscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewTestnetBscscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewPolygonscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewFtmscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewMumbaiPolygonscan() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewTomoBlockExplorer() *EtherscanLikeExplorer {
	return NewEtherscanV2()
}

func NewSnowtrace() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api.routescan.io/v2/network/mainnet/evm/43114/etherscan/",
		"NWT5MMCQMAPYH47DGD8K7QIJGP2DHZRMW3",
	)
}
