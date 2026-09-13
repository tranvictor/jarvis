package explorers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (ee *EtherscanLikeExplorer) GetVerifiedSource(address string) (VerifiedSource, error) {
	addr := strings.TrimSpace(address)
	for _, u := range ee.contractInfoURLs(addr) {
		for attempt := 0; attempt < abiFetchAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(abiRetryDelay)
			}
			src, kind, err := ee.getVerifiedSourceOnce(u, addr)
			if kind == fetchOK {
				return src, nil
			}
			if kind == fetchUnverified {
				return VerifiedSource{Address: addr}, nil
			}
			if kind == fetchRetry {
				_ = err
				continue
			}
			break
		}
	}
	return VerifiedSource{Address: addr}, nil
}

func (ee *EtherscanLikeExplorer) getVerifiedSourceOnce(u, addr string) (VerifiedSource, fetchKind, error) {
	status, body, err := ee.get(u)
	if err != nil {
		return VerifiedSource{}, fetchRetry, err
	}
	if status == http.StatusTooManyRequests || isRateLimited(string(body)) {
		return VerifiedSource{}, fetchRetry, fmt.Errorf("%s: rate limited", ee.label())
	}
	if src, ok := parseEtherscanSource(body, addr); ok {
		if src.Verified && src.Source != "" {
			return src.VerifiedSource, fetchOK, nil
		}
		if src.okEmpty {
			return VerifiedSource{Address: addr}, fetchUnverified, nil
		}
		return src.VerifiedSource, fetchOK, nil
	}
	if src, ok := parseJSONSource(body, addr); ok {
		return src, fetchOK, nil
	}
	if status >= 400 {
		return VerifiedSource{}, fetchMiss, fmt.Errorf("%s: HTTP %d", ee.label(), status)
	}
	return VerifiedSource{}, fetchMiss, fmt.Errorf("%s: unexpected response", ee.label())
}

type etherscanSourceParse struct {
	VerifiedSource
	okEmpty bool
}

func parseEtherscanSource(body []byte, addr string) (etherscanSourceParse, bool) {
	var sc sourceCodeResponse
	if json.Unmarshal(body, &sc) != nil {
		return etherscanSourceParse{}, false
	}
	if sc.Status != "1" && sc.Status != "0" {
		return etherscanSourceParse{}, false
	}
	if sc.Status != "1" || len(sc.Result) == 0 {
		return etherscanSourceParse{okEmpty: true}, true
	}
	r := sc.Result[0]
	code := flattenSource(r.SourceCode)
	verified := code != "" && r.ABI != "Contract source code not verified"
	return etherscanSourceParse{
		VerifiedSource: VerifiedSource{
			Address:        addr,
			Source:         code,
			Verified:       verified,
			Implementation: r.Implementation,
			IsProxy:        r.Proxy == "1",
		},
		okEmpty: !verified,
	}, true
}

func parseJSONSource(body []byte, addr string) (VerifiedSource, bool) {
	info, ok := parseJSONContractInfo(body)
	if !ok {
		return VerifiedSource{}, false
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return VerifiedSource{}, false
	}
	code := flattenSource(jsonString(raw["source_code"]))
	if code == "" {
		code = flattenSource(jsonString(raw["SourceCode"]))
	}
	return VerifiedSource{
		Address:        addr,
		Source:         code,
		Verified:       info.IsVerified && code != "",
		Implementation: info.Implementation,
		IsProxy:        info.IsProxy,
	}, true
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
