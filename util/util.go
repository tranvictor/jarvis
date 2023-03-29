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
	"os/user"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	bleve "github.com/tranvictor/jarvis/bleve"
	. "github.com/tranvictor/jarvis/common"
	db "github.com/tranvictor/jarvis/db"
	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/broadcaster"
	"github.com/tranvictor/jarvis/util/cache"
	"github.com/tranvictor/jarvis/util/monitor"
	"github.com/tranvictor/jarvis/util/reader"
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

func ParamToBigInt(param string) (*big.Int, error) {
	var result *big.Int
	param = strings.Trim(param, " ")
	if len(param) > 2 && param[0:2] == "0x" {
		result = HexToBig(param)
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
	customABIs map[string]*abi.ABI,
	degenMode bool) {
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
		AnalyzeAndPrint(reader, analyzer, t.Hash().Hex(), network, false, "", a, customABIs, degenMode)
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
	customABIs map[string]*abi.ABI,
	degenMode bool) *TxResult {
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

	if degenMode {
		PrintTxDetails(result, network, os.Stdout)
	} else {
		PrintTxSuccessSummary(result, network, os.Stdout)
	}
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
	nodes, err := getCustomNode(network)
	if err != nil {
		nodes = network.GetDefaultNodes()
	}
	customNode := strings.Trim(os.Getenv(network.GetNodeVariableName()), " ")
	if customNode != "" {
		nodes["custom-node"] = customNode
	}
	return nodes, nil
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
		return GetERC20ABI(), nil
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

func IsERC20ABI(a *abi.ABI) bool {
	for _, m := range ERC20_METHODS {
		_, found := a.Methods[m]
		if !found {
			return false
		}
	}
	return true
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

func GetBalances(wallets []string, tokens []string, network Network) (balances map[common.Address][]*big.Int, block int64, err error) {
	return GetHistoryBalances(-1, wallets, tokens, network)
}

func GetHistoryBalances(atBlock int64, wallets []string, tokens []string, network Network) (balances map[common.Address][]*big.Int, block int64, err error) {
	helperABI := GetMultiCallABI()
	erc20ABI := GetERC20ABI()

	mc, err := NewMultiCall(network)
	if err != nil {
		return nil, 0, err
	}

	balances = map[common.Address][]*big.Int{}

	for _, wallet := range wallets {
		wAddr := HexToAddress(wallet)
		for i, token := range tokens {
			index := i
			oneResult := big.NewInt(0)
			balances[wAddr] = append(balances[wAddr], oneResult)
			if strings.ToLower(token) == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" {
				mc.RegisterWithHook(
					&oneResult,
					func(r interface{}) error {
						balances[wAddr][index] = *r.(**big.Int)
						return nil
					},
					network.MultiCallContract(),
					helperABI,
					"getEthBalance",
					HexToAddress(wallet),
				)
			} else {
				mc.RegisterWithHook(
					&oneResult,
					func(r interface{}) error {
						balances[wAddr][index] = *r.(**big.Int)
						return nil
					},
					token,
					erc20ABI,
					"balanceOf",
					HexToAddress(wallet),
				)
			}
		}
	}

	block, err = mc.Do(atBlock)

	return balances, block, err
}

func NewMultiCall(network Network) (*reader.MultipleCall, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return reader.NewMultiCall(r, network.MultiCallContract()), nil
}

func getCustomNode(network Network) (map[string]string, error) {
	usr, _ := user.Current()
	dir := usr.HomeDir
	file := path.Join(dir, "nodes.json")
	fi, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	// if the file is a symlink
	if fi.Mode()&os.ModeSymlink != 0 {
		file, err = os.Readlink(file)
		if err != nil {
			return nil, err
		}
	}
	content, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	result := map[string]map[string]string{}
	err = json.Unmarshal(content, &result)
	if err != nil {
		return nil, err
	}
	node, ok := result[network.GetName()]
	if !ok {
		return nil, fmt.Errorf("Could not get node from custom file")
	}
	return node, nil
}
