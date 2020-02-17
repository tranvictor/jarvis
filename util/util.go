package util

import (
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/monitor"
	"github.com/tranvictor/ethutils/reader"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/tx"
	"github.com/tranvictor/jarvis/txanalyzer"
	"github.com/tranvictor/jarvis/util/cache"
)

const (
	ETH_ADDR                  string = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	ETHEREUM_MAINNET_NODE_VAR string = "ETHEREUM_MAINNET_NODE"
	ETHEREUM_ROPSTEN_NODE_VAR string = "ETHEREUM_ROPSTEN_NODE"
	TOMO_MAINNET_NODE_VAR     string = "TOMO_MAINNET_NODE"
)

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
		mo := monitor.NewTxMonitor()
		mo.BlockingWait(t.Hash().Hex())
		tx.AnalyzeAndPrint(t.Hash().Hex(), network)
	}
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
			"mainnet-alchemy": "https://eth-mainnet.alchemyapi.io/jsonrpc/YP5f6eM2wC9c2nwJfB0DC1LObdSY7Qfv",
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

func VerboseAddress(addr string) string {
	addrDesc, err := db.GetAddress(addr)
	if err != nil {
		return fmt.Sprintf("%s (Unknown)", addr)
	}
	return fmt.Sprintf("%s (%s)", addr, addrDesc.Desc)
}

func GetERC20Decimal(addr string, network string) (int64, error) {
	reader, err := EthReader(network)
	if err != nil {
		return 0, err
	}
	return reader.ERC20Decimal(addr)
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
