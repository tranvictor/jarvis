package explorers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const CACHE_TIME_OUT int64 = 30 // 30 seconds

type EtherscanLikeExplorer struct {
	gpmu              sync.Mutex
	latestGasPrice    float64
	gasPriceTimestamp int64
	ChainID           uint64

	Domain string
	APIKey string
}

func NewEtherscanLikeExplorer(domain string, apiKey string, chainID uint64) *EtherscanLikeExplorer {
	return &EtherscanLikeExplorer{
		gpmu:    sync.Mutex{},
		Domain:  domain,
		APIKey:  apiKey,
		ChainID: chainID,
	}
}

func (ee *EtherscanLikeExplorer) RecommendedGasPriceAPIURL() string {
	return fmt.Sprintf(
		"%s/api?chainid=%dmodule=gastracker&action=gasoracle&apikey=%s",
		ee.Domain,
		ee.ChainID,
		ee.APIKey,
	)
}

type etherscanGasResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Result  struct {
		LastBlock       string `json:"LastBlock"`
		SafeGasPrice    string `json:"SafeGasPrice"`
		ProposeGasPrice string `json:"ProposeGasPrice"`
		FastGasPrice    string `json:"FastGasPrice"`
	} `json:"result"`
}

func (ee *EtherscanLikeExplorer) getGasPrice() (low, average, fast float64, err error) {
	resp, err := http.Get(ee.RecommendedGasPriceAPIURL())
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, 0, err
	}
	prices := etherscanGasResponse{}
	err = json.Unmarshal(body, &prices)
	if err != nil {
		return 0, 0, 0, fmt.Errorf(
			"couldn't unmarshal %s to gas price struct, err: %w",
			string(body),
			err,
		)
	}
	low, err = strconv.ParseFloat(prices.Result.SafeGasPrice, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	average, err = strconv.ParseFloat(prices.Result.ProposeGasPrice, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	fast, err = strconv.ParseFloat(prices.Result.FastGasPrice, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	return low, average, fast, nil
}

func (ee *EtherscanLikeExplorer) RecommendedGasPrice() (float64, error) {
	ee.gpmu.Lock()
	defer ee.gpmu.Unlock()

	if ee.latestGasPrice == 0 || time.Now().Unix()-ee.gasPriceTimestamp > CACHE_TIME_OUT {
		_, _, esFast, err := ee.getGasPrice()
		if err != nil {
			return 0, fmt.Errorf("etherscan gas price lookup failed: %w", err)
		}

		ee.latestGasPrice = esFast
		ee.gasPriceTimestamp = time.Now().Unix()
	}
	return ee.latestGasPrice, nil
}

func (ee *EtherscanLikeExplorer) GetABIStringAPIURL(address string) string {
	return fmt.Sprintf(
		"%s/api?chainid=%d&module=contract&action=getabi&address=%s&apikey=%s",
		ee.Domain,
		ee.ChainID,
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
	var lastErr error
	for attempt := 0; attempt < abiFetchAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(abiRetryDelay)
		}
		result, retry, err := ee.getABIStringOnce(address)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return "", lastErr
}

// getABIStringOnce performs one getabi request. retry is true only for
// transient failures (rate limiting, transport errors).
func (ee *EtherscanLikeExplorer) getABIStringOnce(address string) (result string, retry bool, err error) {
	resp, err := http.Get(ee.GetABIStringAPIURL(address))
	if err != nil {
		return "", true, fmt.Errorf("%s: %w", ee.label(), redactURLError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, fmt.Errorf("%s: reading response: %w", ee.label(), err)
	}
	abiresp := abiresponse{}
	if err := json.Unmarshal(body, &abiresp); err != nil {
		return "", false, fmt.Errorf("%s: unexpected response: %w", ee.label(), err)
	}
	if abiresp.Status != "1" {
		return "", isRateLimited(abiresp.Result), fmt.Errorf("%s: %s", ee.label(), abiresp.Result)
	}
	return abiresp.Result, false, nil
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
		"%s/api?chainid=%d&module=contract&action=getsourcecode&address=%s&apikey=%s",
		ee.Domain,
		ee.ChainID,
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
	resp, err := http.Get(ee.getSourceCodeAPIURL(address))
	if err != nil {
		return ContractInfo{}, fmt.Errorf("%s: %w", ee.label(), redactURLError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ContractInfo{}, fmt.Errorf("%s: reading response: %w", ee.label(), err)
	}
	var sc sourceCodeResponse
	if err := json.Unmarshal(body, &sc); err != nil {
		return ContractInfo{}, fmt.Errorf("unmarshal getsourcecode body: %w", err)
	}
	if sc.Status != "1" || len(sc.Result) == 0 {
		// Etherscan returns Status="0" / Message="NOTOK" for unverified
		// contracts. That's not an error from jarvis's POV — we simply
		// don't have a name to display.
		return ContractInfo{}, nil
	}
	r := sc.Result[0]
	info := ContractInfo{
		Name:           r.ContractName,
		Implementation: r.Implementation,
		IsProxy:        r.Proxy == "1",
		// ABI is the literal string "Contract source code not verified" when
		// the source is missing; treat any other value as verified.
		IsVerified: r.ABI != "" && r.ABI != "Contract source code not verified",
	}
	return info, nil
}
