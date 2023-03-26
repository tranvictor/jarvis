package explorers

type BlockExplorer interface {
	RecommendedGasPrice() (float64, error)
	GetABIString(address string) (string, error)
}

func NewMainnetEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api.etherscan.io",
		"UBB257TI824FC7HUSPT66KZUMGBPRN3IWV",
	)
}

func NewRopstenEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api-ropsten.etherscan.io",
		"UBB257TI824FC7HUSPT66KZUMGBPRN3IWV",
	)
}

func NewRinkebyEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api-rinkeby.etherscan.io",
		"UBB257TI824FC7HUSPT66KZUMGBPRN3IWV",
	)
}

func NewKovanEtherscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api-kovan.etherscan.io",
		"UBB257TI824FC7HUSPT66KZUMGBPRN3IWV",
	)
}

func NewBscscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api.bscscan.com",
		"62TU8Z81F7ESNJT38ZVRBSX7CNN4QZSP5I",
	)
}

func NewTestnetBscscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api-testnet.bscscan.com",
		"62TU8Z81F7ESNJT38ZVRBSX7CNN4QZSP5I",
	)
}

func NewPolygonscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api.polygonscan.com",
		"AE61PSRHNZ7WS1R1BZUXZXDGE52MMNC22U",
	)
}

func NewFtmscan() *EtherscanLikeExplorer {
	return NewEtherscanLikeExplorer(
		"https://api.ftmscan.com",
		"IDMTDBFTDKE7SSHX2V2FYKWWQ9WTVW73HR",
	)
}

func NewMumbaiPolygonscan() *EtherscanLikeExplorer {
	return nil
}

func NewTomoBlockExplorer() *EtherscanLikeExplorer {
	return nil
}
