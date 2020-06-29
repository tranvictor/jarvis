package util

import (
	"fmt"
	"math/big"
	"os"
	"regexp"
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
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util/cache"
)

const (
	ETH_ADDR                  string = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	MAX_ADDR                  string = "0xffffffffffffffffffffffffffffffffffffffff"
	MIN_ADDR                  string = "0x00000000000000ffffffffffffffffffffffffff"
	ETHEREUM_MAINNET_NODE_VAR string = "ETHEREUM_MAINNET_NODE"
	ETHEREUM_ROPSTEN_NODE_VAR string = "ETHEREUM_ROPSTEN_NODE"
	TOMO_MAINNET_NODE_VAR     string = "TOMO_MAINNET_NODE"
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
	case "tomo":
		return time.Duration(uint64(time.Second) * (to - from) * 3)
	}
	panic("unsupported network")
}

func GetAddressFromString(str string) (addr string, name string, err error) {
	addrDesc, err := db.GetAddress(str)
	if err != nil {
		name = "Unknown"
		addresses := ScanForAddresses(str)
		if len(addresses) == 0 {
			return "", "", fmt.Errorf("address not found for \"%s\"", str)
		}
		addr = addresses[0]
	} else {
		name = addrDesc.Desc
		addr = addrDesc.Address
	}
	return addr, name, nil
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

// Split value by space, parse the first element to float64 as the amount.
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

func DisplayWaitAnalyze(t *types.Transaction, broadcasted bool, err error, network string) {
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
		analyzer, err := EthAnalyzer(network)
		if err != nil {
			fmt.Printf("Couldn't analyze the tx: %s\n", err)
			return
		}
		AnalyzeAndPrint(analyzer, t.Hash().Hex(), network)
	}
}

func EthTxMonitor(network string) (*monitor.TxMonitor, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return monitor.NewGenericTxMonitor(r), nil
}

func EthAnalyzer(network string) (*txanalyzer.TxAnalyzer, error) {
	r, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	return txanalyzer.NewGenericAnalyzer(r), nil
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
	case "tomo":
		nodes := map[string]string{
			"mainnet-tomo": "https://rpc.tomochain.com",
		}
		customNode := strings.Trim(os.Getenv(TOMO_MAINNET_NODE_VAR), " ")
		if customNode != "" {
			nodes["custom-node"] = customNode
		}
		return nodes, nil
	}
	return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten, tomo.")
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
	case "tomo":
		return reader.NewTomoReaderWithCustomNodes(nodes), nil
	}
	return nil, fmt.Errorf("Invalid network. Valid values are: mainnet, ropsten, tomo.")
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

func ReadableNumber(value string) string {
	digits := []string{}
	for i, _ := range value {
		digits = append([]string{string(value[len(value)-1-i])}, digits...)
		if (i+1)%3 == 0 && i < len(value)-1 {
			if (i+1)%9 == 0 {
				digits = append([]string{"‸"}, digits...)
			} else {
				digits = append([]string{"￺"}, digits...)
			}
		}
	}
	return fmt.Sprintf("%s (%s)", value, strings.Join(digits, ""))
}

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

func verboseValue(value string, network string) string {
	valueBig, isHex := big.NewInt(0).SetString(value, 0)
	if !isHex {
		return value
	}

	// if it is not a real address
	if !isRealAddress(value) {
		// if this is a hex, it is likely to be a byte data so don't display
		// in readable number
		if len(value) >= 2 && value[0:2] == "0x" {
			return value
		}
		// otherwise, it is a number then return it in a readable format
		return ReadableNumber(value)
	}
	return VerboseAddress(common.BigToAddress(valueBig).Hex(), network)
}

func VerboseValues(values []string, network string) []string {
	result := []string{}
	for _, value := range values {
		result = append(result, verboseValue(value, network))
	}
	return result
}

func DisplayValues(values []string, network string) string {
	verboseValues := VerboseValues(values, network)
	if len(verboseValues) == 0 {
		return ""
	} else if len(verboseValues) == 1 {
		return verboseValues[0]
	} else {
		parts := []string{}
		for i, value := range values {
			parts = append(parts, fmt.Sprintf("%d. %s", i+1, verboseValue(value, network)))
		}
		return strings.Join(parts, "\n")
	}
}

func VerboseAddress(addr string, network string) string {
	var decimal int64
	var erc20Detected bool

	isERC20, err := IsERC20(addr, network)
	if err == nil && isERC20 {
		cacheKey := fmt.Sprintf("%s_decimal", addr)
		decimal, erc20Detected = cache.GetInt64Cache(cacheKey)
	}

	addrDesc, err := db.GetAddress(addr)
	if err != nil {
		return fmt.Sprintf("%s (Unknown)", addr)
	}

	if erc20Detected {
		return fmt.Sprintf("%s (%s)", addr, nameWithColor(fmt.Sprintf("%s - %d", addrDesc.Desc, decimal)))
	} else {
		return fmt.Sprintf("%s (%s)", addr, nameWithColor(addrDesc.Desc))
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

func GetABI(addr string, network string) (*abi.ABI, error) {
	cacheKey := fmt.Sprintf("%s_abi", addr)
	cached, found := cache.GetCache(cacheKey)
	if found {
		result, err := abi.JSON(strings.NewReader(cached))
		if err != nil {
			return nil, err
		}
		return &result, nil
	}

	// not found from cache, getting from etherscan or equivalent websites
	reader, err := EthReader(network)
	if err != nil {
		return nil, err
	}
	abiStr, err := reader.GetABIString(addr)
	if err != nil {
		return nil, err
	}

	result, err := abi.JSON(strings.NewReader(abiStr))
	if err != nil {
		return nil, err
	}

	cache.SetCache(
		cacheKey,
		abiStr,
	)
	return &result, nil
}

func IsGnosisMultisig(addr string, network string) (bool, error) {
	abi, err := GetABI(addr, network)
	if err != nil {
		return false, err
	}
	// loosely check by checking a set of method names

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
		_, found := abi.Methods[m]
		if !found {
			return false, nil
		}
	}
	return true, nil
}
