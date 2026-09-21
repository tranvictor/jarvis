package explorers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// httpClient bounds every explorer request so a hung API cannot stall
// a jarvis command indefinitely. 8s is long enough for a cold CDN and
// short enough that a dead explorer fails the lookup instead of hanging.
var httpClient = &http.Client{Timeout: 8 * time.Second}

// httpCache stores non-rate-limited explorer/Sourcify responses for the
// process lifetime. GetABIString, GetContractInfo, and GetVerifiedSource
// share URL shapes on Blockscout/Robinscan REST, and vet calls Source
// more than once for the same address on one --careful card.
type cachedHTTP struct {
	status int
	body   []byte
}

var httpCache sync.Map // string URL -> cachedHTTP

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

// origin is the explorer host without a trailing /api or /api/v2, used by
// Blockscout and Robinscan REST paths. Etherscan/Routescan never append
// those REST paths onto origin.
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
	return ee.familyURLs(addr, ee.GetABIStringAPIURL(addr), ee.getABIStringAPIURLNoChainID(addr))
}

func (ee *EtherscanLikeExplorer) contractInfoURLs(address string) []string {
	addr := strings.TrimSpace(address)
	return ee.familyURLs(addr, ee.getSourceCodeAPIURL(addr), ee.getSourceCodeAPIURLNoChainID(addr))
}

// familyURLs is the per-Kind lookup list. withChain / withoutChain are the
// Etherscan-compat module=contract URLs for this action (getabi or
// getsourcecode). Other families ignore them or append them as a last try.
func (ee *EtherscanLikeExplorer) familyURLs(addr, withChain, withoutChain string) []string {
	switch ee.kind() {
	case KindRobinscan:
		return []string{ee.origin() + "/api/contracts/" + addr}
	case KindBlockscout:
		return ee.blockscoutURLs(addr, withChain, withoutChain)
	case KindRoutescan:
		// Chain id is already in the Routescan path; extra chainid is a
		// harmless fallback if a gateway requires it.
		return dedupeStrings([]string{withoutChain, withChain})
	default:
		if ee.etherscanV2() {
			return []string{withChain}
		}
		return dedupeStrings([]string{withoutChain, withChain})
	}
}

func (ee *EtherscanLikeExplorer) blockscoutURLs(addr, withChain, withoutChain string) []string {
	origin := ee.origin()
	urls := []string{origin + "/api/v2/smart-contracts/" + addr}
	d := ee.trimmedDomain()
	lower := strings.ToLower(d)
	// Bitfi and some OP-stack Blockscout builds put REST at Domain/api/v2
	// and the singular /smart-contract/:addr path. Do not hit that path
	// on a bare host — it is the HTML contract page, not the API.
	if strings.HasSuffix(lower, "/api/v2") || strings.HasSuffix(lower, "/api/v1") {
		urls = append(urls, d+"/smart-contract/"+addr)
	}
	return dedupeStrings(append(urls, withoutChain, withChain))
}

func (ee *EtherscanLikeExplorer) etherscanV2() bool {
	d := strings.ToLower(ee.trimmedDomain())
	if !etherscanHost(hostOf(d), d) {
		return false
	}
	return strings.Contains(d, "/v2")
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (ee *EtherscanLikeExplorer) get(u string) (int, []byte, error) {
	return getURL(ee.label(), u)
}

func getURL(label, u string) (int, []byte, error) {
	if v, ok := httpCache.Load(u); ok {
		c := v.(cachedHTTP)
		return c.status, c.body, nil
	}
	resp, err := httpClient.Get(u)
	if err != nil {
		wrapped := redactURLError(err)
		if label != "" {
			return 0, nil, fmt.Errorf("%s: %w", label, wrapped)
		}
		return 0, nil, wrapped
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if label != "" {
			return resp.StatusCode, nil, fmt.Errorf("%s: reading response: %w", label, err)
		}
		return resp.StatusCode, nil, fmt.Errorf("reading response: %w", err)
	}
	// Do not cache 429 / rate-limit JSON: the retry loop must re-hit the
	// network after abiRetryDelay, not replay the limited payload.
	if resp.StatusCode != http.StatusTooManyRequests && !isRateLimited(body) {
		httpCache.Store(u, cachedHTTP{status: resp.StatusCode, body: body})
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
	if sc.Status != "1" {
		if isUnverifiedMessage(string(bytes.TrimSpace(sc.Result))) {
			return etherscanContractParse{ok: false}, true
		}
		return etherscanContractParse{}, false
	}
	records := parseSourceRecords(sc.Result)
	if len(records) == 0 {
		return etherscanContractParse{ok: false}, true
	}
	r := records[0]
	verified := r.ABI != "" && r.ABI != "Contract source code not verified"
	impl := strings.TrimSpace(r.Implementation)
	if impl == "" {
		impl = strings.TrimSpace(r.ImplementationAddress)
	}
	info := ContractInfo{
		Name:           r.ContractName,
		Implementation: impl,
		IsProxy:        r.Proxy == "1" || jsonBool(r.IsProxy),
		IsVerified:     verified,
	}
	if verified {
		info.ABI = r.ABI
	}
	return etherscanContractParse{
		info: info,
		ok:   true,
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
		IsVerified: jsonBool(raw["isVerified"]) ||
			jsonBool(raw["is_verified"]) ||
			jsonBool(raw["is_fully_verified"]) ||
			jsonBool(raw["is_partially_verified"]) ||
			jsonBool(raw["is_verified_via_sourcify"]),
		IsProxy: jsonBool(raw["is_proxy"]),
	}
	if abiStr, ok := abiFieldToString(raw["abi"]); ok {
		info.ABI = abiStr
		if !info.IsVerified {
			info.IsVerified = true
		}
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
