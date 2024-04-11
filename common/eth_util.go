package common

import (
	"fmt"
	"io/ioutil"
	"path"
	"runtime"
	"strings"
  "time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

var Start time.Time

func getABIFromFile(filename string) (*abi.ABI, error) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("couldn't get filepath of the caller")
	}
	content, err := ioutil.ReadFile(path.Join(path.Dir(current), filename))
	if err != nil {
		return nil, err
	}

	result, err := abi.JSON(strings.NewReader(string(content)))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func GetMultiCallABI() *abi.ABI {
	result, _ := abi.JSON(strings.NewReader(multicallabi))
	return &result
}

func GetERC20ABI() *abi.ABI {
	result, _ := abi.JSON(strings.NewReader(erc20abi))
	return &result
}

func GetEIP1967BeaconABI() *abi.ABI {
	result, _ := abi.JSON(strings.NewReader(eip1967beacon))
	return &result
}

func PackERC20Data(function string, params ...interface{}) ([]byte, error) {
	return GetERC20ABI().Pack(function, params...)
}

func HexToAddress(hex string) common.Address {
	return common.HexToAddress(hex)
}

func HexToAddresses(hexes []string) []common.Address {
	result := []common.Address{}
	for _, h := range hexes {
		result = append(result, common.HexToAddress(h))
	}
	return result
}

func HexToHash(hex string) common.Hash {
	return common.HexToHash(hex)
}
