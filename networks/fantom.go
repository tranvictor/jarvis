package networks

import (
	"os"
	"strings"
	"time"

	. "github.com/tranvictor/jarvis/util/explorers"
)

var Fantom Network = NewFantom()

type fantom struct {
	*EtherscanLikeExplorer
}

func NewFantom() *fantom {
	result := &fantom{NewFtmscan()}
	apiKey := strings.Trim(os.Getenv(result.GetBlockExplorerAPIKeyVariableName()), " ")
	if apiKey != "" {
		result.EtherscanLikeExplorer.APIKey = apiKey
	}
	return result
}

func (self *fantom) GetName() string {
	return "fantom"
}

func (self *fantom) GetChainID() uint64 {
	return 250
}

func (self *fantom) GetAlternativeNames() []string {
	return []string{"ftm"}
}

func (self *fantom) GetNativeTokenSymbol() string {
	return "FTM"
}

func (self *fantom) GetNativeTokenDecimal() uint64 {
	return 18
}

func (self *fantom) GetBlockTime() time.Duration {
	return 1 * time.Second
}

func (self *fantom) GetNodeVariableName() string {
	return "FANTOM_MAINNET_NODE"
}

func (self *fantom) GetDefaultNodes() map[string]string {
	return map[string]string{
		"fantom": "https://rpc.ftm.tools/",
	}
}

func (self *fantom) GetBlockExplorerAPIKeyVariableName() string {
	return "FTMSCAN_API_KEY"
}

func (self *fantom) GetBlockExplorerAPIURL() string {
	return self.EtherscanLikeExplorer.Domain
}

// func (self *fantom) RecommendedGasPrice() (float64, error) {
// 	res, err := http.Get("https://ftmscan.com/gastracker")
// 	if err != nil {
// 		return 0, err
// 	}
// 	defer res.Body.Close()
//
// 	if res.StatusCode != 200 {
// 		return 0, fmt.Errorf("getting recommended gas price from ftmscan failed: status %d", res.StatusCode)
// 	}
//
// 	// Load the HTML document
// 	doc, err := goquery.NewDocumentFromReader(res.Body)
//
// 	if err != nil {
// 		return 0, fmt.Errorf("ftmscan returned with non html page")
// 	}
//
// 	fastGasSpan := doc.Find("span#fastgas")
// 	if fastGasSpan.Size() == 0 {
// 		return 0, fmt.Errorf("couldn't Contract ABI on avalanche explorer page. The address might not be verified or the explorer page structure changed")
// 	}
//
// 	//fastGasSpan.Text() is in format of "189 gwei"
// 	return strconv.ParseFloat(strings.Split(fastGasSpan.Text(), " ")[0], 64)
// }

func (self *fantom) MultiCallContract() string {
	return "0xcf591ce5574258aC4550D96c545e4F3fd49A74ec"
}
