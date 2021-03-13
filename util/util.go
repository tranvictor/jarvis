package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/broadcaster"
	"github.com/tranvictor/ethutils/monitor"
	"github.com/tranvictor/ethutils/reader"
	bleve "github.com/tranvictor/jarvis/bleve"
	. "github.com/tranvictor/jarvis/common"
	db "github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/util/cache"
)

const (
	ETH_ADDR                  string = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	MAX_ADDR                  string = "0xffffffffffffffffffffffffffffffffffffffff"
	MIN_ADDR                  string = "0x00000000000000ffffffffffffffffffffffffff"
	ETHEREUM_MAINNET_NODE_VAR string = "ETHEREUM_MAINNET_NODE"
	ETHEREUM_ROPSTEN_NODE_VAR string = "ETHEREUM_ROPSTEN_NODE"
	TOMO_MAINNET_NODE_VAR     string = "TOMO_MAINNET_NODE"
	ETHEREUM_KOVAN_NODE_VAR   string = "ETHEREUM_KOVAN_NODE"
	ETHEREUM_RINKEBY_NODE_VAR string = "ETHEREUM_RINKEBY_NODE"
	BSC_MAINNET_NODE_VAR      string = "BSC_MAINNET_NODE"
	BSC_TESTNET_NODE_VAR      string = "BSC_TESTNET_NODE"
)

func CalculateTimeDurationFromBlock(network string, from, to uint64) time.Duration {
	if from >= to {
		return time.Duration(0)
	}
	switch network {
	case "mainnet":
		return time.Duration(uint64(time.Second) * (to - from) * 16)
	case "ropsten":
		return time.Duration(uint64(time.Second) * (to - from) * 16)
	case "kovan":
		return time.Duration(uint64(time.Second) * (to - from) * 4)
	case "rinkeby":
		return time.Duration(uint64(time.Second) * (to - from) * 15)
	case "tomo":
		return time.Duration(uint64(time.Second) * (to - from) * 3)
	case "bsc":
		return time.Duration(uint64(time.Second) * (to - from) * 3)
	case "bsc-test":
		return time.Duration(uint64(time.Second) * (to - from) * 3)
	}
	panic("unsupported network")
}

func GetExactAddressFromDatabases(str string) (addrs []string, names []string, scores []int) {
	addrDescs1, scores1 := bleve.GetAddresses(str)
	for i, addr := range addrDescs1 {
		addrs = append(addrs, addr.Address)
		names = append(names, addr.Desc)
		scores = append(scores, scores1[i])
	}
	return addrs, names, scores
}

func getRelevantAddressesFromDatabases(str string) (addrs []string, names []string, scores []int) {
	addrDescs1, scores1 := bleve.GetAddresses(str)
	addrDescs2, scores2 := db.GetAddresses(str)
	buffer := map[string]bool{}
	for i, addr := range addrDescs1 {
		addrs = append(addrs, addr.Address)
		names = append(names, addr.Desc)
		scores = append(scores, scores1[i])
		buffer[strings.ToLower(addr.Address)] = true
	}
	for i, addr := range addrDescs2 {
		if !buffer[strings.ToLower(addr.Address)] {
			addrs = append(addrs, addr.Address)
			names = append(names, addr.Desc)
			scores = append(scores, scores2[i])
		}
	}
	return addrs, names, scores
}

func getRelevantAddressFromDatabases(str string) (addr string, name string, err error) {
	addrs, names, _ := getRelevantAddressesFromDatabases(str)
	if len(addrs) == 0 {
		return "", "", fmt.Errorf("no address was found for '%s'", str)
	}
	return addrs[0], names[0], nil
}

func GetMatchingAddresses(str string) (addrs []string, names []string, scores []int) {
	addrs, names, scores = getRelevantAddressesFromDatabases(str)
	return addrs, names, scores
}

func GetMatchingAddress(str string) (addr string, name string, err error) {
	return getRelevantAddressFromDatabases(str)
}

func GetAddressFromString(str string) (addr string, name string, err error) {
	addr, name, err = getRelevantAddressFromDatabases(str)
	if err != nil {
		name = "Unknown"
		addresses := ScanForAddresses(str)
		if len(addresses) == 0 {
			return "", "", fmt.Errorf("address not found for \"%s\"", str)
		}
		addr = addresses[0]
	}
	return addr, name, nil
}

func StringToBigInt(str string) (*big.Int, error) {
	result, success := big.NewInt(0).SetString(str, 10)
	if !success {
		return nil, fmt.Errorf("parsed %s to big int failed", str)
	}
	return result, nil
}

func ParamToBigInt(param string) (*big.Int, error) {
	var result *big.Int
	param = strings.Trim(param, " ")
	if len(param) > 2 && param[0:2] == "0x" {
		result = ethutils.HexToBig(param)
	} else {
		idInt, err := strconv.Atoi(param)
		if err != nil {
			return nil, err
		}
		result = big.NewInt(int64(idInt))
	}
	return result, nil
}

// Split value by space,
// if the lowercase of first element is 'all', the amount will be -1, indicating a balance query is needed
// else, parses the first element to float64 as the amount.
// Join whats left by space and trim by space, if it is empty, interpret it
// as ETH.
// Error will not be nil if it fails to proceed all of above steps.
func ValueToAmountAndCurrency(value string) (float64, string, error) {
	parts := strings.Split(value, " ")
	if len(parts) == 0 {
		return 0, "", fmt.Errorf("`%s` is invalid. See help to learn more", value)
	}
	amountStr := parts[0]
	currency := strings.Trim(strings.Join(parts[1:], " "), " ")
	if len(currency) == 0 {
		currency = ETH_ADDR
	}

	if strings.ToLower(strings.Trim(amountStr, " ")) == "all" {
		return -1, currency, nil
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, "", fmt.Errorf(
			"`%s` is not float. See help to learn more", amountStr,
		)
	}
	return amount, currency, nil
}

func ScanForTxs(para string) []string {
	re := regexp.MustCompile("(0x)?[0-9a-fA-F]{64}")
	result := re.FindAllString(para, -1)
	if result == nil {
		return []string{}
	}
	return result
}

func ScanForAddresses(para string) []string {
	re := regexp.MustCompile("0x[0-9a-fA-F]{40}([^0-9a-fA-F]|$)")
	result := re.FindAllString(para, -1)
	if result == nil {
		return []string{}
	}
	for i := 0; i < len(result); i++ {
		result[i] = result[i][0:42]
	}
	return result
}

func IsAddress(addr string) bool {
	_, err := PathToAddress(addr)
	return err == nil
}

func PathToAddress(path string) (string, error) {
	re := regexp.MustCompile("(0x)?[0-9a-fA-F]{40}")
	result := re.FindAllString(path, -1)
	if result == nil {
		return "", fmt.Errorf("invalid filename")
	}
	return result[0], nil
}

func DisplayBroadcastedTx(t *types.Transaction, broadcasted bool, err error, network string) {
	if !broadcasted {
		fmt.Printf("couldn't broadcast tx:\n")
		fmt.Printf("error on nodes: %v\n", err)
	} else {
		fmt.Printf("Broadcasted tx: %s\n", t.Hash().Hex())
	}
}

func DisplayWaitAnalyze(
	reader *reader.EthReader,
	analyzer TxAnalyzer,
	t *types.Transaction,
	broadcasted bool,
	err error,
	network string,
	a *abi.ABI,
	customABIs map[string]*abi.ABI) {
	if !broadcasted {
		fmt.Printf("couldn't broadcast tx:\n")
		fmt.Printf("error on nodes: %v\n", err)
	} else {
		fmt.Printf("Broadcasted tx: %s\n", t.Hash().Hex())
		fmt.Printf("---------Waiting for the tx to be mined---------\n")
		mo, err := EthTxMonitor(network)
		if err != nil {
			fmt.Printf("Couldn't monitor the tx: %s\n", err)
			return
		}
		mo.BlockingWait(t.Hash().Hex())
		AnalyzeAndPrint(reader, analyzer, t.Hash().Hex(), network, false, "", a, customABIs)
	}
}

func AnalyzeMethodCallAndPrint(analyzer TxAnalyzer, abi *abi.ABI, data []byte, customABIs map[string]*abi.ABI, network string) (method string, params []ParamResult, gnosisResult *GnosisResult, err error) {
	method, params, gnosisResult, err = analyzer.AnalyzeMethodCall(abi, data, customABIs)
	if err != nil {
		fmt.Printf("Couldn't analyze method call: %s\n", err)
		return
	}
	fmt.Printf("  Method: %s\n", method)
	fmt.Printf("  Params:\n")
	for _, param := range params {
		fmt.Printf("    %s (%s): %s\n", param.Name, param.Type, DisplayValues(param.Value))
	}
	if gnosisResult != nil {
		PrintGnosis(gnosisResult)
	}
	return
}

func AnalyzeAndPrint(
	reader *reader.EthReader,
	analyzer TxAnalyzer,
	tx string,
	network string,
	forceERC20ABI bool,
	customABI string,
	a *abi.ABI,
	customABIs map[string]*abi.ABI) *TxResult {

	txinfo, err := reader.TxInfoFromHash(tx)
	if err != nil {
		fmt.Printf("getting tx info failed: %s", err)
		return nil
	}

	contractAddress := txinfo.Tx.To().Hex()

	code, err := reader.GetCode(contractAddress)
	if err != nil {
		fmt.Printf("checking tx type failed: %s", err)
		return nil
	}
	isContract := len(code) > 0

	var result *TxResult

	if isContract {
		if a == nil {
			a, err = ConfigToABI(contractAddress, forceERC20ABI, customABI, network)
			if err != nil {
				fmt.Printf("Couldn't get abi for %s: %s\n", contractAddress, err)
				return nil
			}
		}
		result = analyzer.AnalyzeOffline(&txinfo, a, customABIs, true, network)
	} else {
		result = analyzer.AnalyzeOffline(&txinfo, nil, nil, false, network)
	}

	PrintTxDetails(result, os.Stdout)
	return result
}

func EthTxMonitor(network string) (*monitor.TxMonitor, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return monitor.NewGenericTxMonitor(r), nil
}

func GetNodes(network string) (map[string]string, error) {
	switch network {
	case "mainnet":
		nodes := map[string]string{
			"mainnet-alchemy": "https://eth-mainnet.alchemyapi.io/v2/YP5f6eM2wC9c2nwJfB0DC1LObdSY7Qfv",
			"mainnet-infura":  "https://mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
		}
		customNode := strings.Trim(os.Getenv(ETHEREUM_MAINNET_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	case "ropsten":
		nodes := map[string]string{
			"ropsten-infura": "https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
		}
		customNode := strings.Trim(os.Getenv(ETHEREUM_ROPSTEN_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	case "kovan":
		nodes := map[string]string{
			"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
		}
		customNode := strings.Trim(os.Getenv(ETHEREUM_KOVAN_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	case "rinkeby":
		nodes := map[string]string{
			"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
		}
		customNode := strings.Trim(os.Getenv(ETHEREUM_RINKEBY_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	case "tomo":
		nodes := map[string]string{
			"mainnet-tomo": "https://rpc.tomochain.com",
		}
		customNode := strings.Trim(os.Getenv(TOMO_MAINNET_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	case "bsc":
		nodes := map[string]string{
			"binance":  "https://bsc-dataseed.binance.org",
			"defibit":  "https://bsc-dataseed1.defibit.io",
			"ninicoin": "https://bsc-dataseed1.ninicoin.io",
		}
		customNode := strings.Trim(os.Getenv(BSC_MAINNET_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	case "bsc-test":
		nodes := map[string]string{
			"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
			"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
			"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
			"binance4": "https://data-seed-prebsc-2-s2.binance.org:8545",
			"binance5": "https://data-seed-prebsc-1-s3.binance.org:8545",
			"binance6": "https://data-seed-prebsc-2-s3.binance.org:8545",
		}
		customNode := strings.Trim(os.Getenv(BSC_TESTNET_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	}
	return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten, kovan, rinkeby, tomo, bsc, bsc-test.")
}

func EthBroadcaster(network string) (*broadcaster.Broadcaster, error) {
	nodes, err := GetNodes(network)
	if err != nil {
		return nil, err
	}
	return broadcaster.NewGenericBroadcaster(nodes), nil
}

func EthReader(network string) (*reader.EthReader, error) {
	nodes, err := GetNodes(network)
	if err != nil {
		return nil, err
	}
	switch network {
	case "mainnet":
		return reader.NewEthReaderWithCustomNodes(nodes), nil
	case "ropsten":
		return reader.NewRopstenReaderWithCustomNodes(nodes), nil
	case "kovan":
		return reader.NewKovanReaderWithCustomNodes(nodes), nil
	case "rinkeby":
		return reader.NewRinkebyReaderWithCustomNodes(nodes), nil
	case "tomo":
		return reader.NewTomoReaderWithCustomNodes(nodes), nil
	case "bsc":
		return reader.NewBSCReaderWithCustomNodes(nodes), nil
	case "bsc-test":
		return reader.NewBSCTestnetReaderWithCustomNodes(nodes), nil
	}
	return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten, kovan, rinkeby, tomo, bsc, bsc-test.")
}

func queryToCheckERC20(addr string, network string) (bool, error) {
	_, err := GetERC20Decimal(addr, network)
	if err != nil {
		if strings.Contains(fmt.Sprintf("%s", err), "abi: attempting to unmarshall an empty string while arguments are expected") {
			return false, nil
		} else {
			return false, err
		}
	}
	return true, nil
}

func IsERC20(addr string, network string) (bool, error) {
	if !isRealAddress(addr) {
		return false, nil
	}

	cacheKey := fmt.Sprintf("%s_isERC20", addr)
	isERC20, found := cache.GetBoolCache(cacheKey)
	if found {
		return isERC20, nil
	}

	isERC20, err := queryToCheckERC20(addr, network)
	if err != nil {
		return false, err
	}

	cache.SetBoolCache(
		cacheKey,
		isERC20,
	)
	return isERC20, nil
}

// func ReadableNumber(value string) string {
// 	digits := []string{}
// 	for i, _ := range value {
// 		digits = append([]string{string(value[len(value)-1-i])}, digits...)
// 		if (i+1)%3 == 0 && i < len(value)-1 {
// 			if (i+1)%9 == 0 {
// 				digits = append([]string{"‸"}, digits...)
// 			} else {
// 				digits = append([]string{"￺"}, digits...)
// 			}
// 		}
// 	}
// 	return fmt.Sprintf("%s (%s)", value, strings.Join(digits, ""))
// }

func isRealAddress(value string) bool {
	valueBig, isHex := big.NewInt(0).SetString(value, 0)
	if !isHex {
		return false
	}
	maxAddrBig, _ := hexutil.DecodeBig(MAX_ADDR)
	minAddrBig, _ := big.NewInt(0).SetString(MIN_ADDR, 0)
	if valueBig.Cmp(maxAddrBig) > 0 || valueBig.Cmp(minAddrBig) <= 0 {
		return false
	}
	return true
}

func GetJarvisValue(value string, network string) Value {
	valueBig, isHex := big.NewInt(0).SetString(value, 0)
	if !isHex {
		return Value{value, "string", nil}
	}

	// if it is not a real address
	if !isRealAddress(value) {
		return Value{value, "bytes", nil}
	}

	addr := GetJarvisAddress(common.BigToAddress(valueBig).Hex(), network)
	return Value{
		common.BigToAddress(valueBig).Hex(),
		"address",
		&addr,
	}
}

func GetJarvisAddress(addr string, network string) Address {
	var decimal int64
	var erc20Detected bool

	isERC20, err := IsERC20(addr, network)
	if err == nil && isERC20 {
		cacheKey := fmt.Sprintf("%s_decimal", addr)
		decimal, erc20Detected = cache.GetInt64Cache(cacheKey)
	}

	addr, name, err := GetAddressFromString(addr)
	if err != nil {
		return Address{
			Address: addr,
			Desc:    "Unknown",
		}
	}

	if erc20Detected {
		return Address{
			Address: addr,
			Desc:    name,
			Decimal: decimal,
		}
	} else {
		return Address{
			Address: addr,
			Desc:    name,
		}
	}
}

func GetERC20Decimal(addr string, network string) (int64, error) {
	cacheKey := fmt.Sprintf("%s_decimal", addr)
	result, found := cache.GetInt64Cache(cacheKey)
	if found {
		return result, nil
	}

	reader, err := EthReader(network)
	if err != nil {
		return 0, err
	}

	result, err = reader.ERC20Decimal(addr)

	if err != nil {
		return 0, err
	}

	cache.SetInt64Cache(
		cacheKey,
		result,
	)

	return result, nil
}

func isHttpURL(path string) bool {
	u, err := url.ParseRequestURI(path)
	if err != nil {
		return false
	}
	if u.Scheme == "" {
		return false
	}
	return true
}

func ReadCustomABIString(addr string, pathOrAddress string, network string) (str string, err error) {
	if isRealAddress(pathOrAddress) {
		return GetABIString(pathOrAddress, network)
	} else if isHttpURL(pathOrAddress) {
		str, err = GetABIStringFromURL(pathOrAddress)
	} else if str, err = GetABIStringFromFile(pathOrAddress); err != nil {
		str = pathOrAddress
		err = nil
	}

	return str, err
}

type coingeckopriceresponse map[string]map[string]float64

func GetETHPriceInUSD() (float64, error) {
	resp, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=ethereum&vs_currencies=usd&include_market_cap=false&include_24hr_vol=false&include_24hr_change=false&include_last_updated_at=false")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	priceres := coingeckopriceresponse{}
	err = json.Unmarshal(body, &priceres)
	if err != nil {
		return 0, err
	}
	return priceres["ethereum"]["usd"], nil
}

func GetCoinGeckoRateInUSD(token string) (float64, error) {
	if strings.ToLower(token) == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
		return GetETHPriceInUSD()
	}
	resp, err := http.Get(fmt.Sprintf("https://api.coingecko.com/api/v3/simple/token_price/ethereum?contract_addresses=%s&vs_currencies=USD&include_market_cap=false&include_24hr_vol=false&include_24hr_change=false&include_last_updated_at=false", token))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	priceres := coingeckopriceresponse{}
	err = json.Unmarshal(body, &priceres)
	if err != nil {
		return 0, err
	}
	return priceres[strings.ToLower(token)]["usd"], nil
}

func ReadCustomABI(addr string, pathOrAddress string, network string) (a *abi.ABI, err error) {
	str, err := ReadCustomABIString(addr, pathOrAddress, network)
	if err != nil {
		return nil, err
	}

	a, err = GetABIFromString(str)
	if err != nil {
		return a, err
	}

	cacheKey := fmt.Sprintf("%s_abi", addr)
	cache.SetCache(cacheKey, str)
	fmt.Printf("Stored %s abi to cache.\n", addr)
	return a, nil
}

func GetABIStringFromFile(filepath string) (string, error) {
	abiBytes, err := ioutil.ReadFile(filepath)
	return string(abiBytes), err
}

func GetABIStringFromURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	return string(body), err
}

func GetABIFromBytes(abiBytes []byte) (*abi.ABI, error) {
	result, err := abi.JSON(bytes.NewReader(abiBytes))
	return &result, err
}

func GetABIFromString(abiStr string) (*abi.ABI, error) {
	result, err := abi.JSON(strings.NewReader(abiStr))
	return &result, err
}

func GetABIString(addr string, network string) (string, error) {
	cacheKey := fmt.Sprintf("%s_abi", addr)
	cached, found := cache.GetCache(cacheKey)
	if found {
		return cached, nil
	}

	// not found from cache, getting from etherscan or equivalent websites
	reader, err := EthReader(network)
	if err != nil {
		return "", err
	}
	abiStr, err := reader.GetABIString(addr)
	if err != nil {
		return "", err
	}

	cache.SetCache(
		cacheKey,
		abiStr,
	)
	fmt.Printf("Stored %s abi to cache.\n", addr)
	return abiStr, nil
}

func ConfigToABI(address string, forceERC20ABI bool, customABI string, network string) (*abi.ABI, error) {
	if forceERC20ABI {
		return ethutils.GetERC20ABI()
	}
	if customABI != "" {
		return ReadCustomABI(address, customABI, network)
	}
	return GetABI(address, network)
}

func GetABI(addr string, network string) (*abi.ABI, error) {
	abiStr, err := GetABIString(addr, network)
	if err != nil {
		return nil, err
	}

	result, err := GetABIFromString(abiStr)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func IsGnosisMultisig(a *abi.ABI) (bool, error) {
	methods := []string{
		"confirmations",
		"getTransactionCount",
		"isConfirmed",
		"getConfirmationCount",
		"getOwners",
		"transactions",
		"transactionCount",
		"required",
	}

	for _, m := range methods {
		_, found := a.Methods[m]
		if !found {
			return false, nil
		}
	}
	return true, nil
}

func AllZeroParamFunctions(a *abi.ABI) []abi.Method {
	methods := []abi.Method{}
	for _, m := range a.Methods {
		if m.IsConstant() && len(m.Inputs) == 0 {
			methods = append(methods, m)
		}
	}
	sort.Sort(orderedMethods(methods))
	return methods
}
