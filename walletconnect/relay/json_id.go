package relay

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// uint64FromJSONID parses a JSON-RPC "id" value. The Reown relay often
// uses decimal strings (see relay-server-rpc.md examples with "id": "1")
// while our outbound requests use numeric ids. Go's json cannot unmarshal
// a string into uint64, so the read loop would otherwise fail the whole
// connection on the first server push.
func uint64FromJSONID(raw json.RawMessage) (uint64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("empty id")
	}
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return 0, err
		}
		return strconv.ParseUint(str, 10, 64)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return strconv.ParseUint(n.String(), 10, 64)
}
