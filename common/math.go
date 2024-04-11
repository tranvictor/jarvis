package common

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

func HexToBig(hex string) *big.Int {
	result, err := hexutil.DecodeBig(hex)
	if err != nil {
		panic(err)
	}
	return result
}

func FloatToInt(amount float64) int64 {
	s := fmt.Sprintf("%.0f", amount)
	if i, err := strconv.Atoi(s); err == nil {
		return int64(i)
	} else {
		panic(err)
	}
}

// FloatToBigInt converts a float to a big int with specific decimal
// Example:
// - FloatToBigInt(1, 4) = 10000
// - FloatToBigInt(1.234, 4) = 12340
func FloatToBigInt(amount float64, decimal uint64) *big.Int {
	// 9 is our smallest precision, if amount is < 0.000000001 there will be
  // precision loss, the return value will be less than amount * 10^decimal
	if decimal < 9 {
		return big.NewInt(FloatToInt(amount * math.Pow10(int(decimal))))
	}
	result := big.NewInt(FloatToInt(amount * math.Pow10(9)))
	return result.Mul(result, big.NewInt(0).Exp(big.NewInt(10), big.NewInt(int64(decimal-9)), nil))
}

// BigToFloat converts a big int to float according to its number of decimal digits
// Example:
// - BigToFloat(1100, 3) = 1.1
// - BigToFloat(1100, 2) = 11
// - BigToFloat(1100, 5) = 0.11
func BigToFloat(b *big.Int, decimal uint64) float64 {
	f := new(big.Float).SetInt(b)
	power := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(int64(decimal)), nil,
	))
	res := new(big.Float).Quo(f, power)
	result, _ := res.Float64()
	return result
}

func StringToBig(input string) *big.Int {
	resultBig, ok := big.NewInt(0).SetString(input, 10)
	if !ok {
		return big.NewInt(0)
	}
	return resultBig
}

func StringToFloat(input string, decimal uint64) float64 {
	resultBig, ok := big.NewInt(0).SetString(input, 10)
	if !ok {
		return 0.0
	}
	return BigToFloat(resultBig, decimal)
}

// GweiToWei converts Gwei as a float to Wei as a big int
func GweiToWei(n float64) *big.Int {
	return FloatToBigInt(n, 9)
}

// EthToWei converts Gwei as a float to Wei as a big int
func EthToWei(n float64) *big.Int {
	return FloatToBigInt(n, 18)
}

func StringToBigInt(str string) (*big.Int, error) {
	result, success := big.NewInt(0).SetString(str, 10)
	if !success {
		return nil, fmt.Errorf("parsed %s to big int failed", str)
	}
	return result, nil
}

func FloatStringToBig(value string, decimal uint64) (*big.Int, error) {
	f, success := new(big.Float).SetString(value)
	if !success {
		return nil, fmt.Errorf("couldn't parse string to big int")
	}
	power := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(int64(decimal)), nil,
	))
	f.Mul(f, power)
	res, _ := f.Int(nil)
	return res, nil
}

func BigToFloatString(value *big.Int, decimal uint64) string {
	f := new(big.Float).SetInt(value)
	power := new(big.Float).SetInt(new(big.Int).Exp(
		big.NewInt(10), big.NewInt(int64(decimal)), nil,
	))
	res := new(big.Float).Quo(f, power)
	return strings.TrimRight(res.Text('f', int(decimal)), "0")
}
