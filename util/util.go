package util

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tranvictor/ethutils/monitor"
	"github.com/tranvictor/ethutils/reader"
	"github.com/tranvictor/jarvis/db"
	"github.com/tranvictor/jarvis/tx"
	"github.com/tranvictor/jarvis/util/cache"
)

const ETH_ADDR string = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

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
	re := regexp.MustCompile("(0x)?[0-9a-fA-F]{40}")
	result := re.FindAllString(para, -1)
	if result == nil {
		return []string{}
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

func EthReader(network string) (*reader.EthReader, error) {
	switch network {
	case "mainnet":
		return reader.NewEthReader(), nil
	case "ropsten":
		return reader.NewRopstenReader(), nil
	case "tomo":
		return reader.NewTomoReader(), nil
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
