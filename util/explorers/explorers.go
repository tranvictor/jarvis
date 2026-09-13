package explorers

// ContractInfo is the subset of Etherscan-style getsourcecode response the
// rest of jarvis cares about. Implementation is the proxy's underlying
// singleton when applicable, empty otherwise. ABI is the explorer-published
// ABI JSON when the response included one (getsourcecode does); empty when
// the explorer only returned metadata.
type ContractInfo struct {
	Name           string
	Implementation string
	IsProxy        bool
	IsVerified     bool
	ABI            string
}

// VerifiedSource is the Solidity (or flattened multi-file source) the
// explorer published for an address. Used by vet; GetContractInfo stays
// name/proxy/ABI-only so operational lookups do not start carrying source.
type VerifiedSource struct {
	Address        string
	Source         string
	Verified       bool
	Implementation string
	IsProxy        bool
}

type BlockExplorer interface {
	GetABIString(address string) (string, error)
	// GetContractInfo returns the verified-source metadata for a contract:
	// its display name (e.g. "InitializableImmutableAdminUpgradeabilityProxy"),
	// proxy flag, and underlying implementation address when proxied. Used
	// to enrich the address book when the local DB has no entry. Returns
	// IsVerified=false (and no error) when the explorer reports the source
	// as unverified — callers should treat that as "no name available"
	// rather than as a hard failure.
	GetContractInfo(address string) (ContractInfo, error)
	GetVerifiedSource(address string) (VerifiedSource, error)
}
