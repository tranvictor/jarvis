package walletconnect

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// parseSendTxParams pulls the single-element array argument most
// WC-enabled dApps send for eth_sendTransaction and normalises it into
// a RawTx. dApps are inconsistent about quantity encoding (some use
// "0x0", some "0", some unpadded hex), so every numeric field goes
// through parseQuantity.
func parseSendTxParams(raw json.RawMessage) (*RawTx, error) {
	// JSON-RPC params are always an array by spec — in WC v2 it's
	// always a single-element array of the tx object.
	var arr []ethSendTxPayload
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("decode eth_sendTransaction params: %w", err)
	}
	if len(arr) != 1 {
		return nil, fmt.Errorf("eth_sendTransaction expects 1 tx object, got %d", len(arr))
	}
	p := arr[0]

	value, err := parseQuantity(p.Value)
	if err != nil {
		return nil, fmt.Errorf("bad value %q: %w", p.Value, err)
	}
	gas, err := parseUint(p.Gas)
	if err != nil {
		return nil, fmt.Errorf("bad gas %q: %w", p.Gas, err)
	}
	gasPrice, err := parseQuantity(p.GasPrice)
	if err != nil {
		return nil, fmt.Errorf("bad gasPrice %q: %w", p.GasPrice, err)
	}
	maxFee, err := parseQuantity(p.MaxFeePerGas)
	if err != nil {
		return nil, fmt.Errorf("bad maxFeePerGas %q: %w", p.MaxFeePerGas, err)
	}
	maxPrio, err := parseQuantity(p.MaxPriorityFeePerGas)
	if err != nil {
		return nil, fmt.Errorf("bad maxPriorityFeePerGas %q: %w", p.MaxPriorityFeePerGas, err)
	}

	// Prefer Data over Input when both are supplied (rare) — mirrors
	// go-ethereum's tx sanitisation.
	dataField := p.Data
	if dataField == "" {
		dataField = p.Input
	}
	data, err := parseHex(dataField)
	if err != nil {
		return nil, fmt.Errorf("bad data %q: %w", dataField, err)
	}

	rawTx := &RawTx{
		From:                 strings.ToLower(p.From),
		To:                   strings.ToLower(p.To),
		Value:                value,
		Data:                 data,
		Gas:                  gas,
		GasPrice:             gasPrice,
		MaxFeePerGas:         maxFee,
		MaxPriorityFeePerGas: maxPrio,
	}
	if p.Nonce != "" {
		n, err := parseUint(p.Nonce)
		if err != nil {
			return nil, fmt.Errorf("bad nonce %q: %w", p.Nonce, err)
		}
		rawTx.Nonce = n
		rawTx.NonceProvided = true
	}
	return rawTx, nil
}

// parseQuantity decodes an Ethereum JSON-RPC quantity (0x-prefixed
// hex, optionally unpadded). Empty string yields nil *big.Int.
func parseQuantity(s string) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	v := new(big.Int)
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		if _, ok := v.SetString(s[2:], 16); !ok {
			return nil, fmt.Errorf("not a hex quantity")
		}
		return v, nil
	}
	// Some dApps send decimal; accept either.
	if _, ok := v.SetString(s, 10); !ok {
		return nil, fmt.Errorf("not a decimal quantity")
	}
	return v, nil
}

// parseUint is parseQuantity but capped at uint64 for gas / nonce.
func parseUint(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		return strconv.ParseUint(s[2:], 16, 64)
	}
	return strconv.ParseUint(s, 10, 64)
}

// parseHex decodes a 0x-prefixed hex byte string. Empty string or "0x"
// alone decodes to a zero-length slice.
func parseHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if len(s) == 0 {
		return nil, nil
	}
	if len(s)%2 != 0 {
		// Some dApps (famously old MetaMask) emit odd-length hex;
		// pad on the left which matches what parity/geth do.
		s = "0" + s
	}
	return hex.DecodeString(s)
}

// parsePersonalSignParams handles both orderings of
// personal_sign(message, address) that exist in the wild. Most dApps
// use ["<hex|string>", "<address>"], a few legacy ones use
// ["<address>", "<hex|string>"]. We pick the one that looks right,
// falling back to the first form.
//
// The returned bytes are the raw message (already hex-decoded if it
// looked like hex).
func parsePersonalSignParams(raw json.RawMessage) (message []byte, err error) {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("decode personal_sign params: %w", err)
	}
	if len(arr) < 2 {
		return nil, fmt.Errorf("personal_sign expects [message, address], got %d args", len(arr))
	}
	// Determine which arg is the address.
	msgStr := arr[0]
	if isLikelyAddress(arr[0]) && !isLikelyAddress(arr[1]) {
		msgStr = arr[1]
	}
	if strings.HasPrefix(msgStr, "0x") || strings.HasPrefix(msgStr, "0X") {
		b, err := parseHex(msgStr)
		if err != nil {
			return nil, fmt.Errorf("personal_sign hex message: %w", err)
		}
		return b, nil
	}
	return []byte(msgStr), nil
}

// parseSignTypedDataV4Params handles eth_signTypedData_v4's
// [address, typedDataJSON] shape. Returns the raw typedDataJSON bytes
// so the caller can hand them straight to apitypes.TypedData.
func parseSignTypedDataV4Params(raw json.RawMessage) (typedData []byte, err error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("decode eth_signTypedData_v4 params: %w", err)
	}
	if len(arr) < 2 {
		return nil, fmt.Errorf("eth_signTypedData_v4 expects [address, typedData]")
	}
	// Some dApps wrap the typedData as a JSON string, others as an
	// inline object. Detect and unwrap.
	second := arr[1]
	var asStr string
	if err := json.Unmarshal(second, &asStr); err == nil {
		return []byte(asStr), nil
	}
	return second, nil
}

func isLikelyAddress(s string) bool {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

// parseSwitchChainParams handles [{"chainId": "0x<hex>"}].
func parseSwitchChainParams(raw json.RawMessage) (uint64, error) {
	var arr []switchChainPayload
	if err := json.Unmarshal(raw, &arr); err != nil {
		return 0, fmt.Errorf("decode wallet_switchEthereumChain params: %w", err)
	}
	if len(arr) != 1 {
		return 0, fmt.Errorf("wallet_switchEthereumChain expects 1 argument, got %d", len(arr))
	}
	return parseUint(arr[0].ChainID)
}
