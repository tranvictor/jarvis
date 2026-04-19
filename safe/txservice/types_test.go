package txservice

import (
	"encoding/json"
	"testing"
)

func TestNumStringTolerantDecode(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"json string", `{"value":"123"}`, "123"},
		{"json number", `{"value":123}`, "123"},
		{"json zero number", `{"value":0}`, "0"},
		{"json null", `{"value":null}`, ""},
		{"missing field", `{}`, ""},
		{"big uint256 string", `{"value":"115792089237316195423570985008687907853269984665640564039457584007913129639935"}`, "115792089237316195423570985008687907853269984665640564039457584007913129639935"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out struct {
				Value numString `json:"value"`
			}
			if err := json.Unmarshal([]byte(tc.body), &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if out.Value.String() != tc.want {
				t.Fatalf("got %q, want %q", out.Value.String(), tc.want)
			}
		})
	}
}

// TestMultisigTxRealResponse exercises a real-world Safe Transaction Service
// payload that mixes string-form uint256 fields ("value", "gasPrice") with
// number-form integer fields ("safeTxGas", "baseGas"). Regression for the
// unmarshal failure on Aave v3 withdraw transactions.
func TestMultisigTxRealResponse(t *testing.T) {
	body := `{
		"safe":"0x71f8f067348d47cced223eA24D2D77235bea722B",
		"to":"0x87870Bca3F3fD6335C3F4ce8392D69350B4fA4E2",
		"value":"0",
		"data":"0x",
		"operation":0,
		"gasToken":"0x0000000000000000000000000000000000000000",
		"safeTxGas":0,
		"baseGas":0,
		"gasPrice":"0",
		"refundReceiver":"0x0000000000000000000000000000000000000000",
		"nonce":22,
		"safeTxHash":"0x82c28e25b40c865440a0e89fd9578fe62f629f5fafaa0af0587342f7b4b41efe",
		"isExecuted":false,
		"confirmations":[]
	}`
	var mt MultisigTx
	if err := json.Unmarshal([]byte(body), &mt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mt.Value.String() != "0" || mt.SafeTxGas.String() != "0" ||
		mt.BaseGas.String() != "0" || mt.GasPrice.String() != "0" {
		t.Fatalf("decoded numeric fields wrong: %+v", mt)
	}
	if mt.Nonce != 22 {
		t.Fatalf("nonce = %d, want 22", mt.Nonce)
	}
}
