package explorers

import (
	"encoding/json"
	"fmt"
	"io"
)

type OptimisticRollupExplorer struct {
	Domain string
	APIKey string
}

func NewOptimisticRollupExplorer(domain string, apiKey string) *OptimisticRollupExplorer {
	return &OptimisticRollupExplorer{
		Domain: domain,
		APIKey: apiKey,
	}
}

// SmartContract represents the response structure from the HTTP call for smart contract verification details.
type SmartContractResponse struct {
	VerifiedTwinAddressHash    string                `json:"verified_twin_address_hash"`
	IsVerified                 bool                  `json:"is_verified"`
	IsChangedBytecode          bool                  `json:"is_changed_bytecode"`
	IsPartiallyVerified        bool                  `json:"is_partially_verified"`
	IsFullyVerified            bool                  `json:"is_fully_verified"`
	IsVerifiedViaSourcify      bool                  `json:"is_verified_via_sourcify"`
	IsVerifiedViaEthBytecodeDB bool                  `json:"is_verified_via_eth_bytecode_db"`
	IsVyperContract            bool                  `json:"is_vyper_contract"`
	IsSelfDestructed           bool                  `json:"is_self_destructed"`
	CanBeVisualizedViaSol2uml  bool                  `json:"can_be_visualized_via_sol2uml"`
	MinimalProxyAddressHash    string                `json:"minimal_proxy_address_hash"`
	SourcifyRepoURL            string                `json:"sourcify_repo_url"`
	Name                       string                `json:"name"`
	OptimizationEnabled        bool                  `json:"optimization_enabled"`
	OptimizationsRuns          int                   `json:"optimizations_runs"`
	CompilerVersion            string                `json:"compiler_version"`
	EVMVersion                 string                `json:"evm_version"`
	VerifiedAt                 string                `json:"verified_at"`
	ABI                        string                `json:"abi"`
	SourceCode                 string                `json:"source_code"`
	FilePath                   string                `json:"file_path"`
	CompilerSettings           interface{}           `json:"compiler_settings"`
	ConstructorArgs            string                `json:"constructor_args"`
	AdditionalSources          []ContractSource      `json:"additional_sources"`
	DecodedConstructorArgs     []ConstructorArgument `json:"decoded_constructor_args"`
	DeployedBytecode           string                `json:"deployed_bytecode"`
	CreationBytecode           string                `json:"creation_bytecode"`
	ExternalLibraries          []ExternalLibrary     `json:"external_libraries"`
	Language                   string                `json:"language"`
}

type ContractSource struct {
	FilePath   string `json:"file_path"`
	SourceCode string `json:"source_code"`
}

type ConstructorArgument struct {
	// Add fields as needed based on expected structure
}

type ExternalLibrary struct {
	Name        string `json:"name"`
	AddressHash string `json:"address_hash"`
}

func (ee *OptimisticRollupExplorer) GetVerifiedSource(address string) (VerifiedSource, error) {
	sc, err := ee.fetchSmartContract(address)
	if err != nil {
		return VerifiedSource{Address: address}, err
	}
	code := flattenSource(sc.SourceCode)
	if code == "" {
		for _, extra := range sc.AdditionalSources {
			if extra.SourceCode != "" {
				code += fmt.Sprintf("// file: %s\n%s\n", extra.FilePath, extra.SourceCode)
			}
		}
	}
	verified := sc.IsVerified || sc.IsFullyVerified || sc.IsPartiallyVerified
	return VerifiedSource{
		Address:        address,
		Source:         code,
		Verified:       verified && code != "",
		Implementation: sc.MinimalProxyAddressHash,
		IsProxy:        sc.MinimalProxyAddressHash != "",
	}, nil
}

func (ee *OptimisticRollupExplorer) GetABIString(address string) (string, error) {
	sc, err := ee.fetchSmartContract(address)
	if err != nil {
		return "", err
	}
	return sc.ABI, nil
}

func (ee *OptimisticRollupExplorer) GetContractInfo(address string) (ContractInfo, error) {
	sc, err := ee.fetchSmartContract(address)
	if err != nil {
		return ContractInfo{}, err
	}
	return ContractInfo{
		Name:           sc.Name,
		Implementation: sc.MinimalProxyAddressHash,
		IsProxy:        sc.MinimalProxyAddressHash != "",
		IsVerified:     sc.IsVerified || sc.IsFullyVerified || sc.IsPartiallyVerified,
		ABI:            sc.ABI,
	}, nil
}

func (ee *OptimisticRollupExplorer) fetchSmartContract(address string) (SmartContractResponse, error) {
	url := fmt.Sprintf("%s/smart-contract/%s", ee.Domain, address)
	resp, err := httpClient.Get(url)
	if err != nil {
		return SmartContractResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return SmartContractResponse{}, err
	}
	sc := SmartContractResponse{}
	if err := json.Unmarshal(body, &sc); err != nil {
		return SmartContractResponse{}, err
	}
	return sc, nil
}
