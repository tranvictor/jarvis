package reader

import (
	. "github.com/tranvictor/jarvis/util/explorers"
)

func NewBSCReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewBscscan())
}

func NewBSCTestnetReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewTestnetBscscan())
}

func NewKovanReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewKovanEtherscan())
}

func NewRinkebyReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewRinkebyEtherscan())
}

func NewRopstenReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewRopstenEtherscan())
}

func NewKovanReader() *EthReader {
	nodes := map[string]string{
		"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewKovanReaderWithCustomNodes(nodes)
}

func NewRinkebyReader() *EthReader {
	nodes := map[string]string{
		"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewRinkebyReaderWithCustomNodes(nodes)
}

func NewBSCReader() *EthReader {
	nodes := map[string]string{
		"binance":  "https://bsc-dataseed.binance.org",
		"defibit":  "https://bsc-dataseed1.defibit.io",
		"ninicoin": "https://bsc-dataseed1.ninicoin.io",
	}
	return NewBSCReaderWithCustomNodes(nodes)
}

func NewBSCTestnetReader() *EthReader {
	nodes := map[string]string{
		"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
		"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
		"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
	}
	return NewBSCReaderWithCustomNodes(nodes)
}

func NewRopstenReader() *EthReader {
	nodes := map[string]string{
		"ropsten-infura": "https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewRopstenReaderWithCustomNodes(nodes)
}

func NewTomoReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewTomoBlockExplorer())
}

func NewTomoReader() *EthReader {
	nodes := map[string]string{
		"mainnet-tomo": "https://rpc.tomochain.com",
	}
	return NewTomoReaderWithCustomNodes(nodes)
}

func NewMaticReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewPolygonscan())
}

func NewMaticReader() *EthReader {
	nodes := map[string]string{
		"infura": "https://polygon-mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewMaticReaderWithCustomNodes(nodes)
}

func NewMumbaiReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewMumbaiPolygonscan())
}

func NewMumbaiReader() *EthReader {
	nodes := map[string]string{
		"infura": "https://polygon-mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewMumbaiReaderWithCustomNodes(nodes)
}

func NewEthReaderWithCustomNodes(nodes map[string]string) *EthReader {
	return NewEthReaderGeneric(nodes, NewMainnetEtherscan())
}

func NewEthReader() *EthReader {
	nodes := map[string]string{
		"mainnet-alchemy": "https://eth-mainnet.alchemyapi.io/jsonrpc/YP5f6eM2wC9c2nwJfB0DC1LObdSY7Qfv",
		"mainnet-infura":  "https://mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	}
	return NewEthReaderWithCustomNodes(nodes)
}
