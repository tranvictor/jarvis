package safe

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
)

// TxBuilderFile is the JSON export produced by the Safe{Wallet}
// "Transaction Builder" app (app.safe.global → Transaction Builder →
// download). Operators use that UI to assemble a batch of calls against
// verified contracts, then hand the file to jarvis to review, sign with a
// hardware wallet and propose.
//
// The file is self-describing: ChainID says which chain it targets and
// Meta.CreatedFromSafeAddress says which Safe it was built for, so jarvis
// can infer both instead of making the operator repeat them. Both are
// cross-checked when the operator does supply them — a mismatch is always
// a hard error, never a warning, because the same calldata replayed on the
// wrong chain or through the wrong Safe is exactly the class of mistake
// this format should make impossible.
type TxBuilderFile struct {
	Version      string        `json:"version"`
	ChainID      string        `json:"chainId"`
	CreatedAt    int64         `json:"createdAt"`
	Meta         TxBuilderMeta `json:"meta"`
	Transactions []TxBuilderTx `json:"transactions"`
}

// TxBuilderMeta is the file's provenance block. CreatedFromSafeAddress is
// legitimately empty in some exports (older txBuilderVersions, or files
// hand-written by scripts), so an empty value means "unknown", not "zero
// address".
type TxBuilderMeta struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	TxBuilderVersion        string `json:"txBuilderVersion"`
	CreatedFromSafeAddress  string `json:"createdFromSafeAddress"`
	CreatedFromOwnerAddress string `json:"createdFromOwnerAddress"`
	Checksum                string `json:"checksum,omitempty"`
}

// TxBuilderTx is one call in the batch. It comes in two flavours the Safe
// UI produces interchangeably:
//
//   - "custom data": Data holds raw hex calldata and ContractMethod is null.
//   - ABI-driven: ContractMethod describes the function and
//     ContractInputsValues holds one value per input, keyed by input name,
//     in whatever form the user typed into the UI.
//
// A transaction with neither is a plain native-token transfer, which is
// only meaningful when Value is non-zero.
type TxBuilderTx struct {
	To                   string                    `json:"to"`
	Value                string                    `json:"value"`
	Data                 *string                   `json:"data"`
	ContractMethod       *TxBuilderMethod          `json:"contractMethod"`
	ContractInputsValues map[string]TxBuilderValue `json:"contractInputsValues"`
}

// TxBuilderValue is one contractInputsValues entry. The Safe UI usually
// quotes every value — even arrays and tuples, which arrive as JSON text
// inside a JSON string — but it writes bare JSON literals for some types
// (notably `true`/`false` for bool inputs, and unquoted numbers), so this
// cannot simply be a string without rejecting real exports.
//
// Unmarshalling normalises both spellings to the same text: a JSON string is
// unquoted, any other literal is kept verbatim. That way `true` and `"true"`
// reach builderValueToJarvisInput identically, and a bare uint256 above 2^53
// survives because its decimal digits are never routed through float64.
type TxBuilderValue struct {
	text string
	raw  []byte
}

// NewTxBuilderValue builds a value from its textual form, for tests and for
// callers assembling a batch in Go instead of reading a file.
func NewTxBuilderValue(text string) TxBuilderValue {
	return TxBuilderValue{text: text}
}

// String returns the value as text, in the form the Safe UI would have typed
// it — which is what the input normaliser consumes.
func (v TxBuilderValue) String() string {
	return v.text
}

func (v *TxBuilderValue) UnmarshalJSON(data []byte) error {
	v.raw = append([]byte(nil), data...)
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		// Also the null case: unmarshalling null into a string is a no-op,
		// leaving "" — the same thing map[string]string used to yield.
		v.text = s
		return nil
	}
	v.text = strings.TrimSpace(string(data))
	return nil
}

// MarshalJSON re-emits exactly what was read, so serialising a parsed batch
// doesn't rewrite bare literals as strings.
func (v TxBuilderValue) MarshalJSON() ([]byte, error) {
	if len(v.raw) > 0 {
		return append([]byte(nil), v.raw...), nil
	}
	return json.Marshal(v.text)
}

// TxBuilderMethod describes the target function. Inputs are kept as raw
// JSON because they are already in ABI-JSON shape (name/type/internalType
// and, for tuples, nested components) — re-modelling them in Go would only
// create opportunities to drop fields, so instead they are spliced verbatim
// into a synthesised one-method ABI.
type TxBuilderMethod struct {
	Name    string            `json:"name"`
	Payable bool              `json:"payable"`
	Inputs  []json.RawMessage `json:"inputs"`
}

// TxBuilderCall is one encoded entry: the MultiSend-ready call plus the
// synthesised ABI it was encoded with. The ABI is carried along so the
// confirmation screen can decode the call back into a readable function
// signature even when the target contract isn't verified on the block
// explorer — the operator should never have to trust that jarvis encoded
// what the file said.
type TxBuilderCall struct {
	jarviscommon.MultiSendCall
	ABI *abi.ABI
}

// ReadTxBuilderFile loads a Safe transaction-builder export from a path.
func ReadTxBuilderFile(path string) (*TxBuilderFile, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("empty tx builder file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tx builder file %s: %w", path, err)
	}
	return parseTxBuilder(data, path)
}

// ParseTxBuilderJSON parses a Safe transaction-builder export handed over as a
// literal JSON document, for pasting a batch straight onto the command line
// without saving it first.
func ParseTxBuilderJSON(raw string) (*TxBuilderFile, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty tx builder json")
	}
	if !strings.HasPrefix(trimmed, "{") {
		// Catching this here turns a bewildering "invalid character 'x'"
		// from the JSON decoder into an actionable message, since the most
		// likely mistake is passing a path to the json flag.
		return nil, fmt.Errorf(
			"tx builder json must be a JSON object starting with '{'; " +
				"to load a file, use --tx-builder-file instead",
		)
	}
	return parseTxBuilder([]byte(trimmed), "<inline json>")
}

func parseTxBuilder(data []byte, source string) (*TxBuilderFile, error) {
	var f TxBuilderFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse tx builder json %s: %w", source, err)
	}
	if err := f.Validate(); err != nil {
		return nil, fmt.Errorf("tx builder json %s: %w", source, err)
	}
	return &f, nil
}

// Validate checks the structural invariants that must hold before jarvis
// will encode anything. It deliberately rejects rather than skips: an entry
// jarvis can't encode must stop the whole batch, since a partially applied
// batch is worse than no batch.
func (f *TxBuilderFile) Validate() error {
	if len(f.Transactions) == 0 {
		return fmt.Errorf("no transactions in the batch")
	}
	for i, tx := range f.Transactions {
		if tx.To == "" {
			return fmt.Errorf("transaction %d: missing \"to\"", i+1)
		}
		if !common.IsHexAddress(tx.To) {
			return fmt.Errorf("transaction %d: %q is not a valid address", i+1, tx.To)
		}
		if _, err := tx.ValueWei(); err != nil {
			return fmt.Errorf("transaction %d: %w", i+1, err)
		}
		if tx.hasRawData() {
			continue
		}
		if tx.ContractMethod == nil {
			if tx.valueIsZero() {
				return fmt.Errorf(
					"transaction %d: has neither \"data\" nor \"contractMethod\", and no value to transfer",
					i+1,
				)
			}
			continue
		}
		if tx.ContractMethod.Name == "" {
			return fmt.Errorf("transaction %d: contractMethod is missing \"name\"", i+1)
		}
	}
	return nil
}

// ChainIDUint parses the file's chainId, which the Safe UI writes as a
// decimal string.
func (f *TxBuilderFile) ChainIDUint() (uint64, error) {
	s := strings.TrimSpace(f.ChainID)
	if s == "" {
		return 0, fmt.Errorf("missing chainId")
	}
	id, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chainId %q: %w", f.ChainID, err)
	}
	return id, nil
}

// SafeAddress returns the Safe this batch was built for, or "" when the
// file doesn't say.
func (f *TxBuilderFile) SafeAddress() string {
	s := strings.TrimSpace(f.Meta.CreatedFromSafeAddress)
	if s == "" || !common.IsHexAddress(s) {
		return ""
	}
	return common.HexToAddress(s).Hex()
}

// TotalValueWei sums the native-token value of every entry. Useful for
// showing the operator what the batch will move out of the Safe.
func (f *TxBuilderFile) TotalValueWei() (*big.Int, error) {
	total := big.NewInt(0)
	for i, tx := range f.Transactions {
		v, err := tx.ValueWei()
		if err != nil {
			return nil, fmt.Errorf("transaction %d: %w", i+1, err)
		}
		total.Add(total, v)
	}
	return total, nil
}

func (tx *TxBuilderTx) hasRawData() bool {
	if tx.Data == nil {
		return false
	}
	s := strings.TrimSpace(*tx.Data)
	return s != "" && s != "0x"
}

func (tx *TxBuilderTx) valueIsZero() bool {
	v, err := tx.ValueWei()
	return err == nil && v.Sign() == 0
}

// ValueWei parses the entry's value. The Safe UI writes it as a decimal
// string already in wei (never a human-readable float), so it is parsed
// strictly as an integer rather than going through jarvis's token-aware
// converters.
func (tx *TxBuilderTx) ValueWei() (*big.Int, error) {
	s := strings.TrimSpace(tx.Value)
	if s == "" {
		return big.NewInt(0), nil
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid value %q (expected a decimal wei amount)", tx.Value)
	}
	if v.Sign() < 0 {
		return nil, fmt.Errorf("negative value %q", tx.Value)
	}
	return v, nil
}

// EncodeCalls turns every entry into MultiSend-ready calldata. Every entry
// gets Operation 0 (CALL): the transaction-builder format has no per-entry
// operation field, and MultiSendCallOnly — which jarvis delegatecalls for
// batches — rejects anything else by design.
func (f *TxBuilderFile) EncodeCalls(network networks.Network) ([]TxBuilderCall, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}

	calls := make([]TxBuilderCall, 0, len(f.Transactions))
	for i, tx := range f.Transactions {
		call, err := tx.encode(network)
		if err != nil {
			return nil, fmt.Errorf("transaction %d (to %s): %w", i+1, tx.To, err)
		}
		calls = append(calls, call)
	}
	return calls, nil
}

func (tx *TxBuilderTx) encode(network networks.Network) (TxBuilderCall, error) {
	value, err := tx.ValueWei()
	if err != nil {
		return TxBuilderCall{}, err
	}

	call := TxBuilderCall{
		MultiSendCall: jarviscommon.MultiSendCall{
			Operation: 0,
			To:        common.HexToAddress(tx.To),
			Value:     value,
		},
	}

	// Raw calldata wins when present: that's how the Safe UI stores
	// "custom data" entries, and re-deriving it from contractMethod could
	// silently produce something different from what the operator saw.
	if tx.hasRawData() {
		data, err := util.ConvertToBytes(strings.TrimSpace(*tx.Data))
		if err != nil {
			return TxBuilderCall{}, fmt.Errorf("invalid raw data: %w", err)
		}
		call.Data = data
		// contractMethod, when present alongside raw data, is only a
		// display aid — keep it so the confirmation screen can decode.
		if tx.ContractMethod != nil {
			if a, err := tx.ContractMethod.SynthesizeABI(); err == nil {
				call.ABI = a
			}
		}
		return call, nil
	}

	if tx.ContractMethod == nil {
		// Pure native-token transfer.
		return call, nil
	}

	a, err := tx.ContractMethod.SynthesizeABI()
	if err != nil {
		return TxBuilderCall{}, err
	}
	method, ok := a.Methods[tx.ContractMethod.Name]
	if !ok {
		return TxBuilderCall{}, fmt.Errorf(
			"synthesized ABI has no method %q", tx.ContractMethod.Name,
		)
	}

	params := make([]any, 0, len(method.Inputs))
	for _, input := range method.Inputs {
		value, ok := tx.ContractInputsValues[input.Name]
		if !ok {
			return TxBuilderCall{}, fmt.Errorf(
				"contractInputsValues has no value for input %q (%s)",
				input.Name, input.Type.String(),
			)
		}
		raw := value.String()
		normalized, err := builderValueToJarvisInput(input.Type, raw)
		if err != nil {
			return TxBuilderCall{}, fmt.Errorf(
				"input %q (%s) with value %q: %w", input.Name, input.Type.String(), raw, err,
			)
		}
		v, err := util.ConvertParamStrToType(input.Name, input.Type, normalized, network)
		if err != nil {
			return TxBuilderCall{}, fmt.Errorf(
				"input %q (%s) with value %q: %w", input.Name, input.Type.String(), raw, err,
			)
		}
		params = append(params, v)
	}

	data, err := a.Pack(method.Name, params...)
	if err != nil {
		return TxBuilderCall{}, fmt.Errorf("packing %s: %w", method.Name, err)
	}
	call.Data = data
	call.ABI = a
	return call, nil
}

// SynthesizeABI builds a single-method *abi.ABI from the file's
// contractMethod block. The inputs are spliced in verbatim because the Safe
// UI already writes them in ABI-JSON form, which lets go-ethereum do all
// the type parsing (including nested tuple components) for us.
func (m *TxBuilderMethod) SynthesizeABI() (*abi.ABI, error) {
	stateMutability := "nonpayable"
	if m.Payable {
		stateMutability = "payable"
	}
	inputs := m.Inputs
	if inputs == nil {
		inputs = []json.RawMessage{}
	}

	entry := map[string]any{
		"type":            "function",
		"name":            m.Name,
		"inputs":          inputs,
		"outputs":         []any{},
		"stateMutability": stateMutability,
	}
	raw, err := json.Marshal([]any{entry})
	if err != nil {
		return nil, fmt.Errorf("building ABI for %s: %w", m.Name, err)
	}
	a, err := util.GetABIFromString(string(raw))
	if err != nil {
		return nil, fmt.Errorf("building ABI for %s: %w", m.Name, err)
	}
	return a, nil
}

// MergeCallABIs groups the synthesised per-call ABIs by target address so the
// confirmation screen can decode every entry of a batch.
//
// Merging (rather than one entry per call) is required because a batch very
// often hits the same contract several times — four setters on one config
// contract, say — and the analyzer looks a call's ABI up by destination
// address alone. Keeping one ABI per address meant the last entry's
// single-method ABI won and every earlier call into that address rendered as
// "<undecoded call>": its selector simply wasn't in the ABI jarvis had
// installed for that destination.
//
// Methods are keyed by name, with a numeric suffix on collision the same way
// go-ethereum disambiguates overloads; decoding goes by selector, so the key
// only has to be unique, not pretty.
func MergeCallABIs(calls []TxBuilderCall) map[string]*abi.ABI {
	merged := map[string]*abi.ABI{}
	for _, c := range calls {
		if c.ABI == nil {
			continue
		}
		key := strings.ToLower(c.To.Hex())
		into, ok := merged[key]
		if !ok {
			// Copy so merging never mutates the ABI hanging off the call
			// itself, which callers may hold on to per entry.
			clone := *c.ABI
			clone.Methods = make(map[string]abi.Method, len(c.ABI.Methods))
			merged[key] = &clone
			into = &clone
		}
		for _, m := range c.ABI.Methods {
			addMethodToABI(into, m)
		}
	}
	return merged
}

// addMethodToABI inserts m under a free key, skipping it when the same
// signature is already present (the common case: the same method called twice
// in one batch with different arguments).
func addMethodToABI(dst *abi.ABI, m abi.Method) {
	for _, have := range dst.Methods {
		if have.Sig == m.Sig {
			return
		}
	}
	key := m.Name
	for i := 0; ; i++ {
		if _, taken := dst.Methods[key]; !taken {
			break
		}
		key = fmt.Sprintf("%s%d", m.Name, i)
	}
	dst.Methods[key] = m
}
