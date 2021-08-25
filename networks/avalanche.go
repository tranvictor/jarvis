package networks

import (
	"fmt"
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var Avalanche Network = NewAvalanche()

type avalanche struct {
}

func NewAvalanche() *avalanche {
	result := &avalanche{}
	return result
}

func (self *avalanche) GetName() string {
	return "avalanche"
}

func (self *avalanche) GetChainID() int64 {
	return 43114
}

func (self *avalanche) GetAlternativeNames() []string {
	return []string{}
}

func (self *avalanche) GetNativeTokenSymbol() string {
	return "AVAX"
}

func (self *avalanche) GetNativeTokenDecimal() int64 {
	return 18
}

func (self *avalanche) GetBlockTime() time.Duration {
	return 2 * time.Second
}

func (self *avalanche) GetNodeVariableName() string {
	return "AVALANCHE_MAINNET_NODE"
}

func (self *avalanche) GetDefaultNodes() map[string]string {
	return map[string]string{
		"avalanche": "https://api.avax.network/ext/bc/C/rpc",
	}
}

func (self *avalanche) GetBlockExplorerAPIKeyVariableName() string {
	return "POLYGONSCAN_API_KEY"
}

func (self *avalanche) GetBlockExplorerAPIURL() string {
	return "not supported"
}

func (self *avalanche) GetABIString(address string) (string, error) {
	res, err := http.Get(fmt.Sprintf("https://cchain.explorer.avax.network/address/%s/contracts", address))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("getting abi string from avalanche explorer failed: status %d", res.StatusCode)
	}

	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(res.Body)

	if err != nil {
		return "", fmt.Errorf("avalanche explorer returned with non html page")
	}

	// Find the review items
	abiTitleSelection := doc.Find("h3:contains(\"Contract ABI\")")
	if abiTitleSelection.Size() == 0 {
		return "", fmt.Errorf("couldn't Contract ABI on avalanche explorer page. The address might not be verified or the explorer page structure changed")
	}

	codeElem := abiTitleSelection.Parent().Next().Find("code")
	if codeElem.Size() == 0 {
		return "", fmt.Errorf("couldn't code element inside Contract ABI section on avalanche explorer page. The explorer page structure changed")
	}
	return codeElem.Text(), nil
}

func (self *avalanche) RecommendedGasPrice() (float64, error) {
	return 225, nil
}

func (self *avalanche) MultiCallContract() string {
	return "0xa00FB557AA68d2e98A830642DBbFA534E8512E5f"
}
