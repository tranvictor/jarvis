package explorers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type OptimisticRollupExplorer struct {
	Domain string
	APIKey string
}

func NewOptimisticRollupExplorer(domain string, apiKey string) *OptimisticRollupExplorer {
	return &OptimisticRollupExplorer{Domain: domain, APIKey: apiKey}
}

// smartContract is the subset of Blockscout's /smart-contract JSON that
// jarvis reads. Extra fields in the response are ignored.
type smartContract struct {
	ABI                     string `json:"abi"`
	Name                    string `json:"name"`
	MinimalProxyAddressHash string `json:"minimal_proxy_address_hash"`
	IsVerified              bool   `json:"is_verified"`
	IsFullyVerified         bool   `json:"is_fully_verified"`
	IsPartiallyVerified     bool   `json:"is_partially_verified"`
}

func (ee *OptimisticRollupExplorer) fetchSmartContract(address string) (smartContract, error) {
	resp, err := http.Get(fmt.Sprintf("%s/smart-contract/%s", ee.Domain, address))
	if err != nil {
		return smartContract{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return smartContract{}, err
	}
	var sc smartContract
	err = json.Unmarshal(body, &sc)
	return sc, err
}

func (ee *OptimisticRollupExplorer) GetABIString(address string) (string, error) {
	sc, err := ee.fetchSmartContract(address)
	return sc.ABI, err
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
	}, nil
}
