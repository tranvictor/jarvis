package txservice

import (
	"bytes"
	"fmt"
	"strconv"
)

// numString is a uint256-friendly JSON number that tolerates either
// representation the Safe Transaction Service uses across endpoints/fields:
// a JSON string ("0", "123") or a JSON number (0, 123). Internally it stores
// the canonical decimal-string form so callers can hand it straight to
// big.Int.SetString without re-checking the wire shape.
//
// Background: the service serialises uint256 fields (value, gasPrice) as
// strings to preserve precision, but smaller integer fields (safeTxGas,
// baseGas) often come back as raw JSON numbers. Using a single tolerant
// type avoids per-field surprises and lets us decode any future fields
// that flip representation without breaking jarvis.
type numString string

func (n *numString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*n = ""
		return nil
	}
	if data[0] == '"' {
		s, err := strconv.Unquote(string(data))
		if err != nil {
			return fmt.Errorf("numString: invalid quoted number %s: %w", data, err)
		}
		*n = numString(s)
		return nil
	}
	*n = numString(data)
	return nil
}

func (n numString) String() string { return string(n) }

// SafeInfo is the subset of GET /api/v1/safes/{address}/ we consume.
type SafeInfo struct {
	Address   string   `json:"address"`
	Nonce     uint64   `json:"nonce"`
	Threshold uint64   `json:"threshold"`
	Owners    []string `json:"owners"`
	Version   string   `json:"version"`
}

// MultisigTx is the subset of GET /api/v1/multisig-transactions/{safeTxHash}/
// (and similar listing endpoints) we consume. Numeric fields use numString
// so we tolerate both JSON-string and JSON-number forms — the service uses
// strings for uint256 fields (value, gasPrice) but raw numbers for smaller
// fields (safeTxGas, baseGas), and historically has flipped these between
// versions.
type MultisigTx struct {
	Safe                  string         `json:"safe"`
	To                    string         `json:"to"`
	Value                 numString      `json:"value"`
	Data                  *string        `json:"data"`
	Operation             uint8          `json:"operation"`
	SafeTxGas             numString      `json:"safeTxGas"`
	BaseGas               numString      `json:"baseGas"`
	GasPrice              numString      `json:"gasPrice"`
	GasToken              string         `json:"gasToken"`
	RefundReceiver        string         `json:"refundReceiver"`
	Nonce                 uint64         `json:"nonce"`
	SafeTxHash            string         `json:"safeTxHash"`
	IsExecuted            bool           `json:"isExecuted"`
	IsSuccessful          *bool          `json:"isSuccessful"`
	TransactionHash       *string        `json:"transactionHash"`
	ConfirmationsRequired *uint64        `json:"confirmationsRequired"`
	Confirmations         []Confirmation `json:"confirmations"`
}

// Confirmation is a single owner signature attached to a MultisigTx.
type Confirmation struct {
	Owner           string  `json:"owner"`
	SubmissionDate  string  `json:"submissionDate"`
	TransactionHash *string `json:"transactionHash"`
	SignatureType   string  `json:"signatureType"`
	Signature       string  `json:"signature"`
}

// ProposeTxRequest is the JSON body for POST /api/v1/safes/{safe}/multisig-transactions/.
// Numeric fields are strings to match the service's serialization format.
type ProposeTxRequest struct {
	To                    string `json:"to"`
	Value                 string `json:"value"`
	Data                  string `json:"data,omitempty"`
	Operation             uint8  `json:"operation"`
	SafeTxGas             string `json:"safeTxGas"`
	BaseGas               string `json:"baseGas"`
	GasPrice              string `json:"gasPrice"`
	GasToken              string `json:"gasToken"`
	RefundReceiver        string `json:"refundReceiver"`
	Nonce                 string `json:"nonce"`
	ContractTransactionHash string `json:"contractTransactionHash"`
	Sender                string `json:"sender"`
	Signature             string `json:"signature,omitempty"`
	Origin                string `json:"origin,omitempty"`
}

// ConfirmRequest is the JSON body for POST /api/v1/multisig-transactions/{safeTxHash}/confirmations/.
type ConfirmRequest struct {
	Signature string `json:"signature"`
}
