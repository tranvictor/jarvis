package networks

import (
	"github.com/ethereum/go-ethereum/common"
)

const (
	etherscanV2 = "https://api.etherscan.io/v2"
	multicall3  = "0xcA11bde05977b3631167028862bE2a173976CA11"
)

func addr(hex string) common.Address { return common.HexToAddress(hex) }

// eth fills the Etherscan-v2 / ETH / 18-decimal defaults that most bundled
// chains share, then constructs a GenericEtherscanNetwork.
func eth(cfg GenericEtherscanNetworkConfig) Network {
	if cfg.NativeTokenSymbol == "" {
		cfg.NativeTokenSymbol = "ETH"
	}
	if cfg.NativeTokenDecimal == 0 {
		cfg.NativeTokenDecimal = 18
	}
	if cfg.BlockExplorerAPIKeyVariableName == "" {
		cfg.BlockExplorerAPIKeyVariableName = "ETHERSCAN_API_KEY"
	}
	if cfg.BlockExplorerAPIURL == "" {
		cfg.BlockExplorerAPIURL = etherscanV2
	}
	return NewGenericEtherscanNetwork(cfg)
}

var EthereumMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "mainnet", AlternativeNames: []string{"ethereum"}, ChainID: 1, BlockTime: 14,
	NodeVariableName: "ETHEREUM_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"mainnet-kyber":    "https://ethereum-rpc.kyberswap.com",
		"kyber-protection": "https://ethereum-rpc-mev-protection.kyberswap.com",
	},
	MultiCallContractAddress: addr("0xeefba1e63905ef1d7acba5a8513c70307c1ce441"),
})

var Ropsten = eth(GenericEtherscanNetworkConfig{
	Name: "ropsten", ChainID: 3, BlockTime: 14,
	NodeVariableName:         "ETHEREUM_ROPSTEN_NODE",
	DefaultNodes:             map[string]string{"ropsten-infura": "https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5"},
	MultiCallContractAddress: addr("0x53c43764255c17bd724f74c4ef150724ac50a3ed"),
})

var Kovan = eth(GenericEtherscanNetworkConfig{
	Name: "kovan", ChainID: 42, BlockTime: 2,
	NodeVariableName:         "ETHEREUM_KOVAN_NODE",
	DefaultNodes:             map[string]string{"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5"},
	MultiCallContractAddress: addr("0x2cc8688c5f75e365aaeeb4ea8d6a480405a48d2a"),
})

var Rinkeby = eth(GenericEtherscanNetworkConfig{
	Name: "rinkeby", ChainID: 4, BlockTime: 2,
	NodeVariableName:         "ETHEREUM_RINKEBY_NODE",
	DefaultNodes:             map[string]string{"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5"},
	MultiCallContractAddress: addr("0x42ad527de7d4e9d9d011ac45b31d8551f8fe9821"),
})

var BSCMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "bsc", ChainID: 56, NativeTokenSymbol: "BNB", BlockTime: 2,
	NodeVariableName: "BSC_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"kyber":            "https://bsc-rpc.kyberswap.com",
		"kyber-protection": "https://bsc-rpc-mev-protection.kyberswap.com",
		"binance":          "https://bsc-dataseed.binance.org",
		"defibit":          "https://bsc-dataseed1.defibit.io",
		"ninicoin":         "https://bsc-dataseed1.ninicoin.io",
	},
	MultiCallContractAddress: addr("0x41263cba59eb80dc200f3e2544eda4ed6a90e76c"),
})

var BSCTestnet = eth(GenericEtherscanNetworkConfig{
	Name: "bsc-testnet", ChainID: 97, NativeTokenSymbol: "BNB", BlockTime: 2,
	NodeVariableName: "BSC_TESTNET_NODE",
	DefaultNodes: map[string]string{
		"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
		"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
		"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
		"binance4": "https://data-seed-prebsc-2-s2.binance.org:8545",
		"binance5": "https://data-seed-prebsc-1-s3.binance.org:8545",
		"binance6": "https://data-seed-prebsc-2-s3.binance.org:8545",
	},
	MultiCallContractAddress: addr("0xae11C5B5f29A6a25e955F0CB8ddCc416f522AF5C"),
})

var Matic = eth(GenericEtherscanNetworkConfig{
	Name: "matic", AlternativeNames: []string{"polygon"}, ChainID: 137, NativeTokenSymbol: "MATIC", BlockTime: 2,
	NodeVariableName: "MATIC_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"kyber": "https://polygon.kyberengineering.io",
		"drpc":  "https://polygon.drpc.org",
	},
	MultiCallContractAddress: addr("0x11ce4B23bD875D7F5C6a31084f55fDe1e9A87507"),
})

var Avalanche = eth(GenericEtherscanNetworkConfig{
	Name: "avalanche", AlternativeNames: []string{"snowtrace"}, ChainID: 43114, NativeTokenSymbol: "AVAX", BlockTime: 2,
	NodeVariableName: "AVALANCHE_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"kyber":     "https://avalanche-rpc.kyberswap.com",
		"avalanche": "https://api.avax.network/ext/bc/C/rpc",
	},
	BlockExplorerAPIURL:      "https://api.routescan.io/v2/network/mainnet/evm/43114/etherscan/",
	MultiCallContractAddress: addr("0xa00FB557AA68d2e98A830642DBbFA534E8512E5f"),
})

var Fantom = eth(GenericEtherscanNetworkConfig{
	Name: "fantom", AlternativeNames: []string{"ftm"}, ChainID: 250, NativeTokenSymbol: "FTM", BlockTime: 1,
	NodeVariableName:         "FANTOM_MAINNET_NODE",
	DefaultNodes:             map[string]string{"fantom": "https://rpc.ftm.tools/"},
	MultiCallContractAddress: addr("0xcf591ce5574258aC4550D96c545e4F3fd49A74ec"),
})

var OptimismMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "optimism", ChainID: 10, BlockTime: 2,
	NodeVariableName:         "OPTIMISM_MAINNET_NODE",
	DefaultNodes:             map[string]string{"mainnet-optimism": "https://mainnet.optimism.io"},
	MultiCallContractAddress: addr("0xD9bfE9979e9CA4b2fe84bA5d4Cf963bBcB376974"),
})

var ArbitrumMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "arbitrum", ChainID: 42161, BlockTime: 2,
	NodeVariableName: "ARBITRUM_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"kyber":  "https://arbitrum-rpc.kyberswap.com",
		"infura": "https://arb1.arbitrum.io/rpc",
	},
	MultiCallContractAddress: addr("0x80C7DD17B01855a6D2347444a0FCC36136a314de"),
})

var BttcMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "bttc", ChainID: 199, NativeTokenSymbol: "BTT", BlockTime: 2,
	NodeVariableName:         "BTTC_MAINNET_NODE",
	DefaultNodes:             map[string]string{"bt.io": "https://rpc.bt.io"},
	MultiCallContractAddress: addr("0xBF69a56D35B8d6f5A8e0e96B245a72F735751e54"),
})

var EthereumPOW = eth(GenericEtherscanNetworkConfig{
	Name: "ethpow", ChainID: 10001, BlockTime: 14,
	NodeVariableName:         "ETHEREUM_POW_NODE",
	DefaultNodes:             map[string]string{"ethpow-team": "https://mainnet.ethereumpow.org"},
	MultiCallContractAddress: addr("0xeefba1e63905ef1d7acba5a8513c70307c1ce441"),
})

var ScrollMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "scroll", ChainID: 534352, BlockTime: 3,
	NodeVariableName:                "SCROLL_MAINNET_NODE",
	DefaultNodes:                    map[string]string{"public-scroll": "https://rpc.scroll.io"},
	BlockExplorerAPIKeyVariableName: "SCROLLSCAN_API_KEY",
	MultiCallContractAddress:        addr(multicall3),
})

var BaseMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "base", ChainID: 8453, BlockTime: 2,
	NodeVariableName: "BASE_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"kyber":            "https://base-rpc.kyberswap.com",
		"kyber-protection": "https://base-rpc-mev-protection.kyberswap.com",
		"public-base":      "https://mainnet.base.org",
	},
	MultiCallContractAddress: addr(multicall3),
})

var PolygonZkevmMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "polygon-zkevm", ChainID: 1101, BlockTime: 10,
	NodeVariableName:                "POLYGON_ZKEVM_MAINNET_NODE",
	DefaultNodes:                    map[string]string{"public-polygonZkevm": "https://zkevm-rpc.com"},
	BlockExplorerAPIKeyVariableName: "POLYGON_ZKEVMSCAN_API_KEY",
	MultiCallContractAddress:        addr(multicall3),
})

var LineaMainnet = eth(GenericEtherscanNetworkConfig{
	Name: "linea", ChainID: 59144, BlockTime: 2,
	NodeVariableName: "LINEA_MAINNET_NODE",
	DefaultNodes: map[string]string{
		"infura-linea":  "https://linea-mainnet.infura.io/v3/1556a477007b49cda01f9f3df4d97edd",
		"mainnet-linea": "https://rpc.linea.build",
	},
	MultiCallContractAddress: addr(multicall3),
})

var BitfiTestnet = NewGenericOptimismNetwork(GenericEtherscanNetworkConfig{
	Name: "bitfi-testnet", ChainID: 891891, NativeTokenSymbol: "ETH", NativeTokenDecimal: 18, BlockTime: 1,
	NodeVariableName: "BITFI_TESTNET_NODE",
	DefaultNodes: map[string]string{
		"public-bitfi-testnet":  "https://bitfi-ledger-testnet.alt.technology",
		"caliber-bitfi-testnet": " https://rpc2-testnet.bitfi.xyz",
	},
	BlockExplorerAPIKeyVariableName: "BITFI_TESTNET_SCAN_API_KEY",
	BlockExplorerAPIURL:             "https://bitfi-ledger-testnet-explorer.alt.technology/api/v2",
	MultiCallContractAddress:        addr(multicall3),
})
