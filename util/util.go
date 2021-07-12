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
	. "github.com/tranvictor/jarvis/networks"
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
	ETHERSCAN_API_KEY_VAR     string = "ETHERSCAN_API_KEY"
	BSCSCAN_API_KEY_VAR       string = "BSCSCAN_API_KEY"
)

func CalculateTimeDurationFromBlock(network Network, from, to uint64) time.Duration {
	if from >= to {
		return time.Duration(0)
	}
	return time.Duration(uint64(time.Second) * (to - from) * uint64(network.GetBlockTime()))
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

func FloatStringToBig(value string, decimal int64) (*big.Int, error) {
	f, success := new(big.Float).SetString(value)
	if !success {
		return nil, fmt.Errorf("couldn't parse string to big int")
	}
	power := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(decimal), nil,
	))
	f.Mul(f, power)
	res, _ := f.Int(nil)
	return res, nil
}

func BigToFloatString(value *big.Int, decimal int64) string {
	f := new(big.Float).SetInt(value)
	power := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(decimal), nil,
	))
	res := new(big.Float).Quo(f, power)
	return strings.TrimRight(res.Text('f', int(decimal)), "0")
}

// Split value by space,
// if the lowercase of first element is 'all', the amount will be "ALL", indicating a balance query is needed
// else, return the string as the amount.
// Join whats left by space and trim by space, if it is empty, interpret it
// as ETH.
// Error will not be nil if it fails to proceed all of above steps.
func ValueToAmountAndCurrency(value string) (string, string, error) {
	parts := strings.Split(value, " ")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("`%s` is invalid. See help to learn more", value)
	}
	amountStr := parts[0]
	currency := strings.Trim(strings.Join(parts[1:], " "), " ")
	if len(currency) == 0 {
		currency = ETH_ADDR
	}

	if strings.ToLower(strings.Trim(amountStr, " ")) == "all" {
		return "ALL", currency, nil
	}

	return amountStr, currency, nil
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

func DisplayBroadcastedTx(t *types.Transaction, broadcasted bool, err error, network Network) {
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
	network Network,
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

func AnalyzeMethodCallAndPrint(analyzer TxAnalyzer, value *big.Int, destination string, data []byte, customABIs map[string]*abi.ABI, network Network) (fc *FunctionCall) {
	fc = analyzer.AnalyzeFunctionCallRecursively(
		GetABI, value, destination, data, customABIs)
	PrintFunctionCall(fc)
	return fc
}

func AnalyzeAndPrint(
	reader *reader.EthReader,
	analyzer TxAnalyzer,
	tx string,
	network Network,
	forceERC20ABI bool,
	customABI string,
	a *abi.ABI,
	customABIs map[string]*abi.ABI) *TxResult {
	if customABIs == nil {
		customABIs = map[string]*abi.ABI{}
	}

	txinfo, err := reader.TxInfoFromHash(tx)
	if err != nil {
		fmt.Printf("getting tx info failed: %s", err)
		return nil
	}

	if txinfo.Tx.To() == nil {
		return nil
	}
	contractAddress := txinfo.Tx.To().Hex()

	isContract, err := IsContract(contractAddress, network)
	if err != nil {
		fmt.Printf("checking tx type failed: %s", err)
		return nil
	}

	var result *TxResult

	if isContract {
		if a == nil {
			a, err = ConfigToABI(contractAddress, forceERC20ABI, customABI, network)
			if err != nil {
				fmt.Printf("Couldn't get abi for %s: %s\n", contractAddress, err)
				return nil
			}
		}
		customABIs[strings.ToLower(txinfo.Tx.To().Hex())] = a
		result = analyzer.AnalyzeOffline(&txinfo, GetABI, customABIs, true, network)
	} else {
		result = analyzer.AnalyzeOffline(&txinfo, GetABI, nil, false, network)
	}

	PrintTxDetails(result, network, os.Stdout)
	return result
}

func EthTxMonitor(network Network) (*monitor.TxMonitor, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return monitor.NewGenericTxMonitor(r), nil
}

func GetNodes(network Network) (map[string]string, error) {
	nodes := network.GetDefaultNodes()
	customNode := strings.Trim(os.Getenv(network.GetNodeVariableName()), " ")
	if customNode != "" {
		nodes["custom-node"] = customNode
	}
	return nodes, nil
	// switch network {
	// case "mainnet":
	// 	nodes := map[string]string{
	// 		"mainnet-alchemy": "https://eth-mainnet.alchemyapi.io/v2/YP5f6eM2wC9c2nwJfB0DC1LObdSY7Qfv",
	// 		"mainnet-infura":  "https://mainnet.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(ETHEREUM_MAINNET_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// case "ropsten":
	// 	nodes := map[string]string{
	// 		"ropsten-infura": "https://ropsten.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(ETHEREUM_ROPSTEN_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// case "kovan":
	// 	nodes := map[string]string{
	// 		"kovan-infura": "https://kovan.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(ETHEREUM_KOVAN_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// case "rinkeby":
	// 	nodes := map[string]string{
	// 		"rinkeby-infura": "https://rinkeby.infura.io/v3/247128ae36b6444d944d4c3793c8e3f5",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(ETHEREUM_RINKEBY_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// case "tomo":
	// 	nodes := map[string]string{
	// 		"mainnet-tomo": "https://rpc.tomochain.com",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(TOMO_MAINNET_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// case "bsc":
	// 	nodes := map[string]string{
	// 		"binance":  "https://bsc-dataseed.binance.org",
	// 		"defibit":  "https://bsc-dataseed1.defibit.io",
	// 		"ninicoin": "https://bsc-dataseed1.ninicoin.io",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(BSC_MAINNET_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// case "bsc-test":
	// 	nodes := map[string]string{
	// 		"binance1": "https://data-seed-prebsc-1-s1.binance.org:8545",
	// 		"binance2": "https://data-seed-prebsc-2-s1.binance.org:8545",
	// 		"binance3": "https://data-seed-prebsc-1-s2.binance.org:8545",
	// 		"binance4": "https://data-seed-prebsc-2-s2.binance.org:8545",
	// 		"binance5": "https://data-seed-prebsc-1-s3.binance.org:8545",
	// 		"binance6": "https://data-seed-prebsc-2-s3.binance.org:8545",
	// 	}
	// 	customNode := strings.Trim(os.Getenv(BSC_TESTNET_NODE_VAR), " ")
	// 	if customNode != "" {
	// 		nodes["custom-node"] = customNode
	// 	}
	// 	return nodes, nil
	// }
	// return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten, kovan, rinkeby, tomo, bsc, bsc-test.")
}

func EthBroadcaster(network Network) (*broadcaster.Broadcaster, error) {
	nodes, err := GetNodes(network)
	if err != nil {
		return nil, err
	}
	return broadcaster.NewGenericBroadcaster(nodes), nil
}

func EthReader(network Network) (*reader.EthReader, error) {
	var result *reader.EthReader
	var err error
	nodes, err := GetNodes(network)
	if err != nil {
		return nil, err
	}

	result = reader.NewEthReaderGeneric(nodes, network)
	// etherscanAPIKey := strings.Trim(os.Getenv(ETHERSCAN_API_KEY_VAR), " ")
	// if etherscanAPIKey != "" {
	// 	result.SetEtherscanAPIKey(etherscanAPIKey)
	// }

	// bscscanAPIKey := strings.Trim(os.Getenv(BSCSCAN_API_KEY_VAR), " ")
	// if bscscanAPIKey != "" {
	// 	result.SetBSCScanAPIKey(bscscanAPIKey)
	// }
	return result, nil
}

func queryToCheckERC20(addr string, network Network) (bool, error) {
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

func IsERC20(addr string, network Network) (bool, error) {
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

func GetJarvisValue(value string, network Network) Value {
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

func GetJarvisAddress(addr string, network Network) Address {
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

func GetERC20Decimal(addr string, network Network) (int64, error) {
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

func ReadCustomABIString(addr string, pathOrAddress string, network Network) (str string, err error) {
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

func ReadCustomABI(addr string, pathOrAddress string, network Network) (a *abi.ABI, err error) {
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

func GetABIStringBypassCache(addr string, network Network) (string, error) {
	cacheKey := fmt.Sprintf("%s_abi", addr)
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

func IsContract(addr string, network Network) (bool, error) {
	cacheKey := fmt.Sprintf("%s_%s_is_contract", strings.ToLower(addr), network)
	_, found := cache.GetCache(cacheKey)
	if found {
		return true, nil
	}

	reader, err := EthReader(network)
	if err != nil {
		return false, err
	}

	code, err := reader.GetCode(addr)
	if err != nil {
		return false, err
	}

	isContract := len(code) > 0

	if isContract {
		cache.SetCache(
			cacheKey,
			"true",
		)
		fmt.Printf("Stored %s contract code to cache.\n", addr)
	}
	return isContract, nil
}

func GetABIString(addr string, network Network) (string, error) {
	cacheKey := fmt.Sprintf("%s_abi", strings.ToLower(addr))
	cached, found := cache.GetCache(cacheKey)
	if found {
		return cached, nil
	}
	return GetABIStringBypassCache(addr, network)
}

func ConfigToABI(address string, forceERC20ABI bool, customABI string, network Network) (*abi.ABI, error) {
	if forceERC20ABI {
		return ethutils.GetERC20ABI()
	}
	if customABI != "" {
		return ReadCustomABI(address, customABI, network)
	}
	return GetABI(address, network)
}

func GetGnosisMsigDeployByteCode(ctorBytes []byte) ([]byte, error) {
	bytecode, err := hexutil.Decode(gnosisMsigDeployCode)
	if err != nil {
		return []byte{}, err
	}
	data := append(bytecode, ctorBytes...)
	return data, nil
}

func GetGnosisMsigABI() *abi.ABI {
	result, err := abi.JSON(strings.NewReader(gnosisMsigABI))
	if err != nil {
		panic(err)
	}
	return &result
}

func GetABI(addr string, network Network) (*abi.ABI, error) {
	abiStr, err := GetABIString(addr, network)
	if err != nil {
		return nil, err
	}

	result, err := GetABIFromString(abiStr)
	if err == nil {
		return result, nil
	}

	// now abiStr is an invalid abi string
	// try bypassing the cache and query again
	abiStr, err = GetABIStringBypassCache(addr, network)
	if err != nil {
		return nil, err
	}
	return GetABIFromString(abiStr)
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

// func (self *EthReader) ReadContractToBytes(atBlock int64, from string, caddr string, abi *abi.ABI, method string, args ...interface{}) ([]byte, error) {

type multicallres struct {
	BlockNumber *big.Int
	ReturnData  [][]byte
}

type call struct {
	Target   common.Address
	CallData []byte
}

func GetBalances(wallets []string, tokens []string, network Network) (balances map[common.Address][]*big.Int, block int64, err error) {
	return GetHistoryBalances(-1, wallets, tokens, network)
}

func GetHistoryBalances(atBlock int64, wallets []string, tokens []string, network Network) (balances map[common.Address][]*big.Int, block int64, err error) {
	multicallContract := network.MultiCallContract()
	if multicallContract == "" {
		return nil, 0, fmt.Errorf("network not support get multi balances")
	}

	helperABI, err := GetABI(multicallContract, network)
	if err != nil {
		return nil, 0, err
	}

	erc20ABI, err := ethutils.GetERC20ABI()
	if err != nil {
		return nil, 0, err
	}

	results := []interface{}{}
	caddrs := []string{}
	abis := []*abi.ABI{}
	methods := []string{}
	argLists := [][]interface{}{}
	for _, wallet := range wallets {
		for _, token := range tokens {
			oneResult := big.NewInt(0)
			results = append(results, &oneResult)
			if strings.ToLower(token) == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
				caddrs = append(caddrs, multicallContract)
				abis = append(abis, helperABI)
				methods = append(methods, "getEthBalance")
				argLists = append(argLists, []interface{}{
					ethutils.HexToAddress(wallet),
				})
			} else {
				caddrs = append(caddrs, token)
				abis = append(abis, erc20ABI)
				methods = append(methods, "balanceOf")
				argLists = append(argLists, []interface{}{
					ethutils.HexToAddress(wallet),
				})
			}
		}
	}

	block, err = MultiReadContract(
		network,
		atBlock,
		results,
		caddrs,
		abis,
		methods,
		argLists,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("multi read contract failed: %w", err)
	}

	balances = map[common.Address][]*big.Int{}
	for i, wallet := range wallets {
		oneWalletBalances := []*big.Int{}
		for j, _ := range tokens {
			oneWalletBalances = append(
				oneWalletBalances,
				*results[i*len(tokens)+j].(**big.Int),
			)
		}
		balances[ethutils.HexToAddress(wallet)] = oneWalletBalances
	}

	return balances, block, nil
}

func MultiReadContract(
	network Network,
	atBlock int64,
	results []interface{},
	caddrs []string,
	abis []*abi.ABI,
	methods []string,
	argLists [][]interface{},
) (block int64, err error) {

	reader, err := EthReader(network)
	if err != nil {
		return 0, err
	}

	contract := network.MultiCallContract()
	if contract == "" {
		return 0, fmt.Errorf("network not support multicall")
	}
	res := multicallres{}

	calls := []call{}
	for i, caddr := range caddrs {
		data, err := abis[i].Pack(methods[i], argLists[i]...)
		if err != nil {
			return 0, err
		}

		calls = append(calls, call{ethutils.HexToAddress(caddr), data})
	}

	err = reader.ReadHistoryContract(
		atBlock,
		&res,
		contract,
		"aggregate",
		calls,
	)

	if err != nil {
		return 0, fmt.Errorf("reading multical failed: %w", err)
	}

	for i, _ := range results {
		err := abis[i].UnpackIntoInterface(
			results[i],
			methods[i],
			res.ReturnData[i],
		)
		if err != nil {
			return 0, fmt.Errorf("unpacking call index %d failed: %w", i, err)
		}
	}
	return res.BlockNumber.Int64(), nil
}
