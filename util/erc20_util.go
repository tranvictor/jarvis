package util

import (
	"fmt"
	"strings"

	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util/cache"
)

var ERC20_METHODS = [...]string{
	"name",
	"symbol",
	"decimals",
	"totalSupply",
	"balanceOf",
	"transfer",
	"transferFrom",
	"approve",
	"allowance",
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

func GetERC20Symbol(addr string, network Network) (string, error) {
	cacheKey := fmt.Sprintf("%s_symbol", addr)
	result, found := cache.GetCache(cacheKey)
	if found {
		return result, nil
	}

	reader, err := EthReader(network)
	if err != nil {
		return "", err
	}

	result, err = reader.ERC20Symbol(addr)

	if err != nil {
		return "", err
	}

	cache.SetCache(
		cacheKey,
		result,
	)

	return result, nil
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
