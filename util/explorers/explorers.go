package explorers

// ContractInfo is the subset of Etherscan-style getsourcecode response the
// rest of jarvis cares about. Implementation is the proxy's underlying
// singleton when applicable, empty otherwise.
type ContractInfo struct {
	Name           string
	Implementation string
	IsProxy        bool
	IsVerified     bool
}

type BlockExplorer interface {
	RecommendedGasPrice() (float64, error)
	GetABIString(address string) (string, error)
	// GetContractInfo returns the verified-source metadata for a contract:
	// its display name (e.g. "InitializableImmutableAdminUpgradeabilityProxy"),
	// proxy flag, and underlying implementation address when proxied. Used
	// to enrich the address book when the local DB has no entry. Returns
	// IsVerified=false (and no error) when the explorer reports the source
	// as unverified — callers should treat that as "no name available"
	// rather than as a hard failure.
	GetContractInfo(address string) (ContractInfo, error)
}
