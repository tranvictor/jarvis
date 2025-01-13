package explorers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
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

func NewEtherscanLikeExplorer(domain string, apiKey string) *EtherscanLikeExplorer {
	return &EtherscanLikeExplorer{
		gpmu:   sync.Mutex{},
		Domain: domain,
		APIKey: apiKey,
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

func (ar *abiresponse) IsOK() bool {
	return ar.Status == "1"
}

func (ee *EtherscanLikeExplorer) GetABIString(address string) (string, error) {
	url := ee.GetABIStringAPIURL(address)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	abiresp := abiresponse{}
	err = json.Unmarshal(body, &abiresp)
	if err != nil {
		return "", err
	}
	if abiresp.Status != "1" {
		return "", fmt.Errorf("error from %s: %s", url, abiresp.Message)
	}
	return abiresp.Result, err
}
