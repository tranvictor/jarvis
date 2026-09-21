package explorers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// sourceRecord is one getsourcecode result row. Etherscan v2 returns an
// array; Blockscout's etherscan-compat endpoint sometimes returns a single
// object. SimilarMatch is Etherscan's bytecode-similar verified twin;
// verified_twin_address_hash is the Blockscout equivalent.
type sourceRecord struct {
	ContractName            string          `json:"ContractName"`
	ABI                     string          `json:"ABI"`
	Proxy                   string          `json:"Proxy"`
	IsProxy                 json.RawMessage `json:"IsProxy"`
	Implementation          string          `json:"Implementation"`
	ImplementationAddress   string          `json:"ImplementationAddress"`
	SourceCode              string          `json:"SourceCode"`
	SimilarMatch            string          `json:"SimilarMatch"`
	VerifiedTwinAddressHash string          `json:"verified_twin_address_hash"`
}

type fetchedSource struct {
	VerifiedSource
	similar string
}

var sourceCache sync.Map // resultKey -> VerifiedSource

func (ee *EtherscanLikeExplorer) GetVerifiedSource(address string) (VerifiedSource, error) {
	addr := strings.TrimSpace(address)
	key := ee.resultKey(addr)
	if v, ok := sourceCache.Load(key); ok {
		return v.(VerifiedSource), nil
	}
	src, err := ee.getVerifiedSource(addr, 0)
	if err == nil {
		sourceCache.Store(key, src)
	}
	return src, err
}

func (ee *EtherscanLikeExplorer) resultKey(addr string) string {
	return strings.Join([]string{
		string(ee.kind()),
		strings.ToLower(ee.Domain),
		fmt.Sprintf("%d", ee.ChainID),
		strings.ToLower(strings.TrimSpace(addr)),
	}, "|")
}

func (ee *EtherscanLikeExplorer) getVerifiedSource(addr string, depth int) (VerifiedSource, error) {
	var lastErr error
	sawUnverified := false
	similar := ""

	for _, u := range ee.contractInfoURLs(addr) {
		got, kind, err := ee.getVerifiedSourceWithRetry(u, addr)
		switch kind {
		case fetchOK:
			return got.VerifiedSource, nil
		case fetchUnverified:
			if got.similar != "" {
				similar = got.similar
			}
			sawUnverified = true
		case fetchRetry, fetchMiss:
			lastErr = err
		}
		if sawUnverified {
			// This family answered. Other URL shapes (v1 chainid,
			// Blockscout etherscan-compat) would repeat the same
			// unverified row; Sourcify / similar-match still run.
			break
		}
	}

	if src, ok := sourcifySource(ee.ChainID, addr); ok {
		return src, nil
	}
	if similar != "" && depth == 0 && !sameHexAddr(similar, addr) {
		if twin, err := ee.getVerifiedSource(similar, depth+1); err == nil && twin.Verified && twin.Source != "" {
			twin.Address = addr
			return twin, nil
		}
	}
	if sawUnverified {
		return VerifiedSource{Address: addr}, nil
	}
	if lastErr != nil {
		return VerifiedSource{Address: addr}, lastErr
	}
	return VerifiedSource{Address: addr}, nil
}

func (ee *EtherscanLikeExplorer) getVerifiedSourceWithRetry(u, addr string) (fetchedSource, fetchKind, error) {
	var last fetchedSource
	var lastKind fetchKind
	var lastErr error
	for attempt := 0; attempt < abiFetchAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(abiRetryDelay)
		}
		src, kind, err := ee.getVerifiedSourceOnce(u, addr)
		if kind == fetchRetry {
			last, lastKind, lastErr = src, kind, err
			continue
		}
		return src, kind, err
	}
	return last, lastKind, lastErr
}

func (ee *EtherscanLikeExplorer) getVerifiedSourceOnce(u, addr string) (fetchedSource, fetchKind, error) {
	status, body, err := ee.get(u)
	if err != nil {
		return fetchedSource{}, fetchRetry, err
	}
	if status == http.StatusTooManyRequests || isRateLimited(body) {
		return fetchedSource{}, fetchRetry, fmt.Errorf("%s: rate limited", ee.label())
	}
	if src, ok := parseEtherscanSource(body, addr); ok {
		if src.Verified && src.Source != "" {
			return src, fetchOK, nil
		}
		return src, fetchUnverified, nil
	}
	if src, ok := parseJSONSource(body, addr); ok {
		if src.Verified && src.Source != "" {
			return src, fetchOK, nil
		}
		return src, fetchUnverified, nil
	}
	if status >= 400 {
		return fetchedSource{}, fetchMiss, fmt.Errorf("%s: HTTP %d", ee.label(), status)
	}
	return fetchedSource{}, fetchMiss, fmt.Errorf("%s: unexpected response", ee.label())
}

func parseEtherscanSource(body []byte, addr string) (fetchedSource, bool) {
	var sc sourceCodeResponse
	if json.Unmarshal(body, &sc) != nil {
		return fetchedSource{}, false
	}
	if sc.Status != "1" && sc.Status != "0" {
		return fetchedSource{}, false
	}
	result := bytes.TrimSpace(sc.Result)
	if len(result) == 0 {
		return fetchedSource{VerifiedSource: VerifiedSource{Address: addr}}, sc.Status == "1"
	}
	if result[0] == '"' {
		var msg string
		if json.Unmarshal(result, &msg) != nil {
			return fetchedSource{}, false
		}
		if isUnverifiedMessage(msg) {
			return fetchedSource{VerifiedSource: VerifiedSource{Address: addr}}, true
		}
		// Invalid API key, missing chainid, etc. — not "this contract is unverified".
		return fetchedSource{}, false
	}
	records := parseSourceRecords(result)
	if sc.Status != "1" || len(records) == 0 {
		if sc.Status == "0" && isUnverifiedMessage(string(result)) {
			return fetchedSource{VerifiedSource: VerifiedSource{Address: addr}}, true
		}
		if sc.Status != "1" {
			return fetchedSource{}, false
		}
		return fetchedSource{VerifiedSource: VerifiedSource{Address: addr}}, true
	}
	return recordToSource(records[0], addr), true
}

func parseSourceRecords(result json.RawMessage) []sourceRecord {
	result = bytes.TrimSpace(result)
	if len(result) == 0 {
		return nil
	}
	if result[0] == '{' {
		var one sourceRecord
		if json.Unmarshal(result, &one) != nil {
			return nil
		}
		return []sourceRecord{one}
	}
	var many []sourceRecord
	if json.Unmarshal(result, &many) != nil {
		return nil
	}
	return many
}

func recordToSource(r sourceRecord, addr string) fetchedSource {
	code := flattenSource(r.SourceCode)
	abiUnverified := r.ABI == "Contract source code not verified"
	verified := code != "" && !abiUnverified
	impl := strings.TrimSpace(r.Implementation)
	if impl == "" {
		impl = strings.TrimSpace(r.ImplementationAddress)
	}
	similar := strings.TrimSpace(r.SimilarMatch)
	if similar == "" {
		similar = strings.TrimSpace(r.VerifiedTwinAddressHash)
	}
	return fetchedSource{
		VerifiedSource: VerifiedSource{
			Address:        addr,
			Source:         code,
			Verified:       verified,
			Implementation: impl,
			IsProxy:        r.Proxy == "1" || jsonBool(r.IsProxy),
		},
		similar: similar,
	}
}

func isUnverifiedMessage(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "not verified")
}

func parseJSONSource(body []byte, addr string) (fetchedSource, bool) {
	info, ok := parseJSONContractInfo(body)
	if !ok {
		return fetchedSource{}, false
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return fetchedSource{}, false
	}
	code := flattenJSONSources(raw)
	similar := jsonString(raw["verified_twin_address_hash"])
	if similar == "" {
		similar = jsonString(raw["SimilarMatch"])
	}
	verified := info.IsVerified && code != ""
	return fetchedSource{
		VerifiedSource: VerifiedSource{
			Address:        addr,
			Source:         code,
			Verified:       verified,
			Implementation: info.Implementation,
			IsProxy:        info.IsProxy,
		},
		similar: similar,
	}, true
}

func flattenJSONSources(raw map[string]json.RawMessage) string {
	var parts []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			parts = append(parts, s)
		}
	}
	main := flattenSource(jsonString(raw["source_code"]))
	if main == "" {
		main = flattenSource(jsonString(raw["SourceCode"]))
	}
	if main != "" {
		if name := jsonString(raw["file_path"]); name != "" {
			add(fmt.Sprintf("// file: %s\n%s", name, main))
		} else {
			add(main)
		}
	}
	add(flattenSourceFiles(raw["sourceFiles"]))
	add(flattenSourceFiles(raw["source_files"]))
	add(flattenSourceFiles(raw["additional_sources"]))
	add(flattenSourceFiles(raw["additionalSources"]))
	if mapped := flattenSourceMap(raw["sources"]); mapped != "" && main == "" {
		add(mapped)
	}
	return strings.Join(parts, "\n")
}

func flattenSourceMap(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' {
		return ""
	}
	var files map[string]struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(raw, &files) != nil || len(files) == 0 {
		return ""
	}
	var b strings.Builder
	n := 0
	for name, f := range files {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		fmt.Fprintf(&b, "// file: %s\n%s\n", name, f.Content)
		n++
	}
	if n == 0 {
		return ""
	}
	return b.String()
}

func flattenSource(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "Contract source code not verified" {
		return ""
	}
	s := raw
	if strings.HasPrefix(s, "{{") && strings.HasSuffix(s, "}}") {
		s = s[1 : len(s)-1]
	}
	if !strings.HasPrefix(s, "{") {
		return raw
	}
	var std struct {
		Sources map[string]struct {
			Content string `json:"content"`
		} `json:"sources"`
	}
	if json.Unmarshal([]byte(s), &std) == nil && len(std.Sources) > 0 {
		var b strings.Builder
		for name, f := range std.Sources {
			fmt.Fprintf(&b, "// file: %s\n%s\n", name, f.Content)
		}
		return b.String()
	}
	var files map[string]string
	if json.Unmarshal([]byte(s), &files) == nil && len(files) > 0 {
		var b strings.Builder
		for name, content := range files {
			fmt.Fprintf(&b, "// file: %s\n%s\n", name, content)
		}
		return b.String()
	}
	return raw
}

// flattenSourceFiles reads a JSON array of source files. Robinscan uses
// {path, content}; Blockscout v2 uses {file_path, source_code}.
func flattenSourceFiles(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '{' {
		return flattenSource(string(raw))
	}
	if raw[0] != '[' {
		return ""
	}
	var files []struct {
		Path       string `json:"path"`
		Name       string `json:"name"`
		File       string `json:"file"`
		FilePath   string `json:"file_path"`
		Content    string `json:"content"`
		SourceCode string `json:"source_code"`
	}
	if json.Unmarshal(raw, &files) != nil || len(files) == 0 {
		return ""
	}
	var b strings.Builder
	n := 0
	for _, f := range files {
		content := f.Content
		if strings.TrimSpace(content) == "" {
			content = f.SourceCode
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		name := f.Path
		if name == "" {
			name = f.FilePath
		}
		if name == "" {
			name = f.Name
		}
		if name == "" {
			name = f.File
		}
		if name == "" {
			name = "source"
		}
		fmt.Fprintf(&b, "// file: %s\n%s\n", name, content)
		n++
	}
	if n == 0 {
		return ""
	}
	return b.String()
}

func sameHexAddr(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
