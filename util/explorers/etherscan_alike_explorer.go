package explorers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type EtherscanLikeExplorer struct {
	ChainID uint64

	Domain string
	APIKey string
}

func NewEtherscanLikeExplorer(domain string, apiKey string, chainID uint64) *EtherscanLikeExplorer {
	return &EtherscanLikeExplorer{
		Domain:  domain,
		APIKey:  apiKey,
		ChainID: chainID,
	}
}

func (ee *EtherscanLikeExplorer) GetABIStringAPIURL(address string) string {
	return fmt.Sprintf(
		"%s?chainid=%d&module=contract&action=getabi&address=%s&apikey=%s",
		ee.apiEndpoint(),
		ee.ChainID,
		address,
		ee.APIKey,
	)
}

func (ee *EtherscanLikeExplorer) getABIStringAPIURLNoChainID(address string) string {
	return fmt.Sprintf(
		"%s?module=contract&action=getabi&address=%s&apikey=%s",
		ee.apiEndpoint(),
		address,
		ee.APIKey,
	)
}

// gas station response
type abiresponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  string `json:"result"`
}

// abiFetchAttempts and abiRetryDelay bound the retry loop around Etherscan's
// per-second rate limit. A single `jarvis info` can look up a dozen ABIs in a
// burst, so one short pause usually turns "rate limit reached" into a hit.
var (
	abiFetchAttempts = 3
	abiRetryDelay    = 400 * time.Millisecond
)

// label names the explorer in errors without echoing the request URL, which
// carries the API key.
func (ee *EtherscanLikeExplorer) label() string {
	return fmt.Sprintf("%s (chain %d)", ee.Domain, ee.ChainID)
}

func isRateLimited(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "rate limit")
}

func (ee *EtherscanLikeExplorer) GetABIString(address string) (string, error) {
	return ee.getABIString(address, 0)
}

func (ee *EtherscanLikeExplorer) getABIString(address string, depth int) (string, error) {
	var lastErr error
	for _, u := range ee.abiURLs(address) {
		result, impl, err := ee.getABIStringWithRetry(u)
		if err == nil {
			if depth < 2 && impl != "" && !strings.EqualFold(impl, address) && !abiJSONHasFunctions(result) {
				if implABI, implErr := ee.getABIString(impl, depth+1); implErr == nil && abiJSONHasFunctions(implABI) {
					return implABI, nil
				}
			}
			return result, nil
		}
		lastErr = err
		if isUnverifiedABI(err) {
			return "", err
		}
	}
	return "", lastErr
}

func (ee *EtherscanLikeExplorer) getABIStringWithRetry(u string) (string, string, error) {
	var lastErr error
	for attempt := 0; attempt < abiFetchAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(abiRetryDelay)
		}
		result, impl, retry, err := ee.getABIStringOnce(u)
		if err == nil {
			return result, impl, nil
		}
		lastErr = err
		if !retry {
			return "", "", err
		}
	}
	return "", "", lastErr
}

// getABIStringOnce performs one ABI request. retry is true only for
// transient failures (rate limiting, transport errors).
func (ee *EtherscanLikeExplorer) getABIStringOnce(u string) (result, impl string, retry bool, err error) {
	status, body, err := ee.get(u)
	if err != nil {
		return "", "", true, err
	}
	if status == http.StatusTooManyRequests || isRateLimited(string(body)) {
		return "", "", true, fmt.Errorf("%s: %s", ee.label(), strings.TrimSpace(string(body)))
	}
	if abiStr, ok := parseABIFromBody(body); ok {
		return abiStr, implementationFromBody(body), false, nil
	}
	abiresp := abiresponse{}
	if json.Unmarshal(body, &abiresp) == nil && abiresp.Status != "" && abiresp.Status != "1" {
		msg := abiresp.Result
		if msg == "" {
			msg = abiresp.Message
		}
		return "", "", isRateLimited(msg), fmt.Errorf("%s: %s", ee.label(), msg)
	}
	if status >= 400 {
		return "", "", false, fmt.Errorf("%s: HTTP %d", ee.label(), status)
	}
	return "", "", false, fmt.Errorf("%s: unexpected response", ee.label())
}

// redactURLError strips the request URL (and with it the API key) out of
// net/http transport errors, keeping only the underlying cause.
func redactURLError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}

func (ee *EtherscanLikeExplorer) getSourceCodeAPIURL(address string) string {
	return fmt.Sprintf(
		"%s?chainid=%d&module=contract&action=getsourcecode&address=%s&apikey=%s",
		ee.apiEndpoint(),
		ee.ChainID,
		address,
		ee.APIKey,
	)
}

func (ee *EtherscanLikeExplorer) getSourceCodeAPIURLNoChainID(address string) string {
	return fmt.Sprintf(
		"%s?module=contract&action=getsourcecode&address=%s&apikey=%s",
		ee.apiEndpoint(),
		address,
		ee.APIKey,
	)
}

// sourceCodeResponse is the v2 Etherscan-multichain getsourcecode shape.
// Many fields are omitted; we only keep what's needed to build ContractInfo.
// Note: Etherscan returns numeric flag fields ("1" / "0") as JSON strings.
type sourceCodeResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  []struct {
		ContractName    string `json:"ContractName"`
		ABI             string `json:"ABI"`
		Proxy           string `json:"Proxy"`
		Implementation  string `json:"Implementation"`
		CompilerVersion string `json:"CompilerVersion"`
	} `json:"result"`
}

func (ee *EtherscanLikeExplorer) GetContractInfo(address string) (ContractInfo, error) {
	for _, u := range ee.contractInfoURLs(address) {
		for attempt := 0; attempt < abiFetchAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(abiRetryDelay)
			}
			info, kind, err := ee.getContractInfoOnce(u)
			if kind == fetchOK {
				return info, nil
			}
			if kind == fetchUnverified {
				// Etherscan returns Status="0" / Message="NOTOK" for unverified
				// contracts. That's not an error from jarvis's POV — we simply
				// don't have a name to display.
				return ContractInfo{}, nil
			}
			if kind == fetchRetry {
				_ = err
				continue
			}
			break
		}
	}
	return ContractInfo{}, nil
}

func (ee *EtherscanLikeExplorer) getContractInfoOnce(u string) (ContractInfo, fetchKind, error) {
	status, body, err := ee.get(u)
	if err != nil {
		return ContractInfo{}, fetchRetry, err
	}
	if status == http.StatusTooManyRequests || isRateLimited(string(body)) {
		return ContractInfo{}, fetchRetry, fmt.Errorf("%s: rate limited", ee.label())
	}
	if info, ok := parseEtherscanContractInfo(body); ok {
		if info.ok {
			return info.info, fetchOK, nil
		}
		return ContractInfo{}, fetchUnverified, nil
	}
	if info, ok := parseJSONContractInfo(body); ok {
		return info, fetchOK, nil
	}
	if status >= 400 {
		return ContractInfo{}, fetchMiss, fmt.Errorf("%s: HTTP %d", ee.label(), status)
	}
	return ContractInfo{}, fetchMiss, fmt.Errorf("%s: unexpected response", ee.label())
}
