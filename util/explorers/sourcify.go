package explorers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// sourcifyContractURL is the Sourcify v2 contract lookup. Tests override it.
// Sourcify is the Verifier Alliance exact-match database: bytecode-equal
// source independent of any one explorer's similar-match / rate-limit
// behaviour. GET .../{chainId}/{address}?fields=sources
var sourcifyContractURL = func(chainID uint64, addr string) string {
	return fmt.Sprintf("https://sourcify.dev/server/v2/contract/%d/%s?fields=sources", chainID, addr)
}

func sourcifySource(chainID uint64, addr string) (VerifiedSource, bool) {
	addr = strings.TrimSpace(addr)
	if chainID == 0 || addr == "" {
		return VerifiedSource{}, false
	}
	u := sourcifyContractURL(chainID, addr)
	status, body, err := getURL("", u)
	if err != nil || status != http.StatusOK {
		return VerifiedSource{}, false
	}
	code := parseSourcifySources(body)
	if code == "" {
		return VerifiedSource{}, false
	}
	return VerifiedSource{
		Address:  addr,
		Source:   code,
		Verified: true,
	}, true
}

func parseSourcifySources(body []byte) string {
	var wrap struct {
		Match   string `json:"match"`
		Sources map[string]struct {
			Content string `json:"content"`
		} `json:"sources"`
	}
	if json.Unmarshal(body, &wrap) != nil || len(wrap.Sources) == 0 {
		return ""
	}
	// match is "match" / "exact_match" / "partial" for a verified contract;
	// a 404 payload has match:null and no sources.
	if wrap.Match == "" || strings.EqualFold(wrap.Match, "null") {
		return ""
	}
	var b strings.Builder
	for name, f := range wrap.Sources {
		if strings.TrimSpace(f.Content) == "" {
			continue
		}
		fmt.Fprintf(&b, "// file: %s\n%s\n", name, f.Content)
	}
	return b.String()
}
