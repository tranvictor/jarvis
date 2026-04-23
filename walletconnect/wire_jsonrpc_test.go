package walletconnect

import (
	"encoding/json"
	"testing"
)

func TestIdUint64FromRaw(t *testing.T) {
	cases := []struct {
		raw string
		out uint64
	}{
		{`1`, 1},
		{`"99"`, 99},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			var raw json.RawMessage
			_ = json.Unmarshal([]byte(c.raw), &raw)
			n, err := idUint64FromRaw(raw)
			if err != nil {
				t.Fatal(err)
			}
			if n != c.out {
				t.Fatalf("got %d want %d", n, c.out)
			}
		})
	}
}
