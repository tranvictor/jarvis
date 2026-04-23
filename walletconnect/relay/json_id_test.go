package relay

import (
	"encoding/json"
	"testing"
)

func TestUint64FromJSONID(t *testing.T) {
	cases := []struct {
		raw string
		out uint64
	}{
		{`1`, 1},
		{`"42"`, 42},
		{`"1740000000000123456"`, 1740000000000123456},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			var raw json.RawMessage
			_ = json.Unmarshal([]byte(c.raw), &raw)
			n, err := uint64FromJSONID(raw)
			if err != nil {
				t.Fatal(err)
			}
			if n != c.out {
				t.Fatalf("got %d want %d", n, c.out)
			}
		})
	}
}
