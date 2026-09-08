package explorers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// fetchKind classifies one explorer HTTP response so callers can decide
// whether to retry, stop, or try a different URL shape.
type fetchKind int

const (
	fetchOK fetchKind = iota
	fetchRetry
	fetchUnverified
	fetchMiss
)

func (ee *EtherscanLikeExplorer) trimmedDomain() string {
	return strings.TrimRight(strings.TrimSpace(ee.Domain), "/")
}

// apiEndpoint is the Etherscan-style module/action root. Built-in networks
// store "https://api.etherscan.io/v2" and we append "/api". Custom networks
// often store the URL the operator copied from the explorer, which already
// ends in "/api" or "/api/v2" — appending another "/api" 404s.
func (ee *EtherscanLikeExplorer) apiEndpoint() string {
	d := ee.trimmedDomain()
	lower := strings.ToLower(d)
	if strings.HasSuffix(lower, "/api") ||
		strings.HasSuffix(lower, "/api/v1") ||
		strings.HasSuffix(lower, "/api/v2") {
		return d
	}
	return d + "/api"
}

// origin is the explorer host without a trailing /api or /api/v2, used for
// JSON REST paths such as Robinscan /api/contracts/:address and Blockscout
// /api/v2/smart-contracts/:address.
func (ee *EtherscanLikeExplorer) origin() string {
	d := ee.trimmedDomain()
	lower := strings.ToLower(d)
	for _, suffix := range []string{"/api/v2", "/api/v1", "/api"} {
		if strings.HasSuffix(lower, suffix) {
			return d[:len(d)-len(suffix)]
		}
	}
	return d
}

func (ee *EtherscanLikeExplorer) abiURLs(address string) []string {
	addr := strings.TrimSpace(address)
	return []string{
		ee.GetABIStringAPIURL(addr),
		ee.getABIStringAPIURLNoChainID(addr),
		ee.origin() + "/api/contracts/" + addr,
		ee.origin() + "/api/v2/smart-contracts/" + addr,
	}
}

func (ee *EtherscanLikeExplorer) contractInfoURLs(address string) []string {
	addr := strings.TrimSpace(address)
	return []string{
		ee.getSourceCodeAPIURL(addr),
		ee.getSourceCodeAPIURLNoChainID(addr),
		ee.origin() + "/api/contracts/" + addr,
		ee.origin() + "/api/v2/smart-contracts/" + addr,
	}
}

func (ee *EtherscanLikeExplorer) get(u string) (int, []byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", ee.label(), redactURLError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("%s: reading response: %w", ee.label(), err)
	}
	return resp.StatusCode, body, nil
}

func isUnverifiedABI(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not verified")
}

func parseABIFromBody(body []byte) (string, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", false
	}
	if trimmed[0] == '[' {
		var arr []json.RawMessage
		if json.Unmarshal(trimmed, &arr) == nil {
			return string(trimmed), true
		}
		return "", false
	}

	var etherscan abiresponse
	if json.Unmarshal(trimmed, &etherscan) == nil && etherscan.Status == "1" {
		if etherscan.Result != "" && etherscan.Result != "Contract source code not verified" {
			return etherscan.Result, true
		}
		return "", false
	}

	var generic map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &generic); err != nil {
		return "", false
	}
	raw, ok := generic["abi"]
	if !ok {
		return "", false
	}
	return abiFieldToString(raw)
}

func abiFieldToString(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		if s == "" || s == "Contract source code not verified" {
			return "", false
		}
		return s, true
	}
	if raw[0] == '[' {
		return string(raw), true
	}
	return "", false
}

func implementationFromBody(body []byte) string {
	info, ok := parseJSONContractInfo(body)
	if !ok {
		return ""
	}
	return strings.TrimSpace(info.Implementation)
}

// abiJSONHasFunctions reports whether an ABI JSON array contains any
// function entries. Transparent proxies verify as constructor + events +
// fallback, so a methodless ABI next to an `implementation` field should
// be replaced by the implementation ABI.
func abiJSONHasFunctions(abiStr string) bool {
	var arr []map[string]any
	if json.Unmarshal([]byte(abiStr), &arr) != nil {
		return true
	}
	for _, item := range arr {
		t, _ := item["type"].(string)
		if strings.EqualFold(t, "function") {
			return true
		}
	}
	return false
}

type etherscanContractParse struct {
	info ContractInfo
	ok   bool
}

func parseEtherscanContractInfo(body []byte) (etherscanContractParse, bool) {
	var sc sourceCodeResponse
	if json.Unmarshal(body, &sc) != nil {
		return etherscanContractParse{}, false
	}
	if sc.Status != "1" && sc.Status != "0" {
		return etherscanContractParse{}, false
	}
	if sc.Status != "1" || len(sc.Result) == 0 {
		return etherscanContractParse{ok: false}, true
	}
	r := sc.Result[0]
	return etherscanContractParse{
		info: ContractInfo{
			Name:           r.ContractName,
			Implementation: r.Implementation,
			IsProxy:        r.Proxy == "1",
			IsVerified:     r.ABI != "" && r.ABI != "Contract source code not verified",
		},
		ok: true,
	}, true
}

func parseJSONContractInfo(body []byte) (ContractInfo, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return ContractInfo{}, false
	}
	if _, hasVerified := raw["isVerified"]; !hasVerified {
		if _, hasSnake := raw["is_verified"]; !hasSnake {
			if _, hasProxy := raw["is_proxy"]; !hasProxy {
				if _, hasABI := raw["abi"]; !hasABI {
					return ContractInfo{}, false
				}
			}
		}
	}

	info := ContractInfo{
		Name:           jsonString(raw["name"]),
		Implementation: parseImplementation(raw["implementation"]),
		IsVerified:     jsonBool(raw["isVerified"]) || jsonBool(raw["is_verified"]),
		IsProxy:        jsonBool(raw["is_proxy"]),
	}
	if proxyType := jsonString(raw["proxyType"]); proxyType != "" && !strings.EqualFold(proxyType, "null") {
		info.IsProxy = true
	}
	if info.Implementation == "" {
		info.Implementation = parseImplementationsArray(raw["implementations"])
	}
	if info.Implementation != "" {
		info.IsProxy = true
	}
	return info, true
}

func jsonString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func jsonBool(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == "1" || strings.EqualFold(s, "true")
	}
	return false
}

func parseImplementation(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		return jsonString(raw)
	}
	var obj struct {
		Address string `json:"address"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return strings.TrimSpace(obj.Address)
	}
	return ""
}

func parseImplementationsArray(raw json.RawMessage) string {
	var arr []struct {
		Address string `json:"address"`
	}
	if json.Unmarshal(raw, &arr) != nil || len(arr) == 0 {
		return ""
	}
	return strings.TrimSpace(arr[0].Address)
}
