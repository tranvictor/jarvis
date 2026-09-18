package explorers

// NewOptimisticRollupExplorer returns a Blockscout-class explorer.
// OP-stack and other rollup explorers historically used Blockscout's
// /api/v2/smart-contract/:address REST; KindBlockscout tries that singular
// path (when Domain already ends in /api/v2), the plural /smart-contracts/
// route, and etherscan-compat module=contract.
func NewOptimisticRollupExplorer(domain, apiKey string, chainID uint64) *EtherscanLikeExplorer {
	return New(KindBlockscout, domain, apiKey, chainID)
}
