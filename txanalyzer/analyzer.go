package txanalyzer

import (
	"bytes"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	. "github.com/tranvictor/jarvis/common"
	. "github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	reader "github.com/tranvictor/jarvis/util/reader"
)

type TxAnalyzer struct {
	ctx *AnalysisContext
}

func (self *TxAnalyzer) setBasicTxInfo(txinfo TxInfo, result *TxResult) {
	result.From = self.ctx.GetJarvisAddress(txinfo.Tx.Extra.From.Hex())
	result.Value = BigToFloatString(txinfo.Tx.Value(), self.ctx.Network.GetNativeTokenDecimal())
	if to := txinfo.Tx.To(); to != nil {
		result.To = self.ctx.GetJarvisAddress(to.Hex())
	} else if txinfo.Receipt != nil {
		// Contract creation: the receipt carries the deployed address.
		result.To = self.ctx.GetJarvisAddress(txinfo.Receipt.ContractAddress.Hex())
	}
	result.Nonce = fmt.Sprintf("%d", txinfo.Tx.Nonce())
	result.GasPrice = fmt.Sprintf("%.4f", BigToFloat(txinfo.Tx.GasPrice(), 9))
	result.GasLimit = fmt.Sprintf("%d", txinfo.Tx.Gas())
	result.GasUsed = fmt.Sprintf("%d", txinfo.Receipt.GasUsed)
	result.GasCost = fmt.Sprintf("%.8f", BigToFloat(txinfo.GasCost(), self.ctx.Network.GetNativeTokenDecimal()))
	if txinfo.Receipt != nil && txinfo.Receipt.BlockNumber != nil {
		result.BlockNumber = txinfo.Receipt.BlockNumber.String()
	}
}

// nonArrayParamAsJarvisValue converts a scalar ABI value to a jarvis Value,
// setting Kind directly from the ABI type so the display layer never has to
// re-guess it from the raw string. When hint is non-nil the contract is a
// known ERC20 token and integer params are annotated as DisplayToken.
func (self *TxAnalyzer) nonArrayParamAsJarvisValue(t abi.Type, value interface{}, hint *ERC20Info) Value {
	switch t.T {
	case abi.StringTy:
		return Value{Raw: value.(string), Kind: DisplayRaw}

	case abi.BoolTy:
		return Value{Raw: fmt.Sprintf("%t", value.(bool)), Kind: DisplayRaw}

	case abi.AddressTy:
		addr := self.ctx.GetJarvisAddress(value.(common.Address).Hex())
		return Value{Raw: addr.Address, Kind: DisplayAddress, Address: &addr}

	case abi.HashTy:
		return Value{Raw: value.(common.Hash).Hex(), Kind: DisplayRaw}

	case abi.IntTy, abi.UintTy:
		raw := fmt.Sprintf("%d", value)
		if hint != nil {
			return Value{
				Raw:  raw,
				Kind: DisplayToken,
				Token: &TokenHint{
					Decimal: hint.Decimal,
					Symbol:  hint.Symbol,
				},
			}
		}
		return Value{Raw: raw, Kind: DisplayInteger}

	case abi.BytesTy:
		return Value{Raw: "0x" + common.Bytes2Hex(value.([]byte)), Kind: DisplayRaw}

	case abi.FixedBytesTy:
		word := make([]byte, reflect.TypeOf(value).Size())
		reflect.Copy(reflect.ValueOf(word), reflect.ValueOf(value))
		return Value{Raw: "0x" + common.Bytes2Hex(word), Kind: DisplayRaw}

	case abi.FunctionTy:
		return Value{Raw: "0x" + common.Bytes2Hex(value.([]byte)), Kind: DisplayRaw}

	default:
		return Value{Raw: fmt.Sprintf("%v", value), Kind: DisplayRaw}
	}
}

func (ta *TxAnalyzer) paramAsJarvisTuple(t abi.Type, value interface{}, hint *ERC20Info) TupleParamResult {
	result := TupleParamResult{
		Name: t.TupleRawName,
		Type: t.String(),
	}

	realVal, ok := value.(reflect.Value)
	if !ok {
		realVal = reflect.ValueOf(value)
	}

	for i, field := range t.TupleElems {
		result.Values = append(result.Values, ta.paramAsJarvisParamResult(
			t.TupleRawNames[i],
			*field,
			reflect.Indirect(realVal).FieldByName(
				cases.Title(language.Und, cases.NoLower).String(t.TupleRawNames[i]),
			).Interface(),
			hint,
		))
	}
	return result
}

// ParamAsJarvisTuple is the public interface method; it delegates to the
// internal variant with no token hint (nil context).
func (ta *TxAnalyzer) ParamAsJarvisTuple(t abi.Type, value interface{}) TupleParamResult {
	return ta.paramAsJarvisTuple(t, value, nil)
}

func (ta *TxAnalyzer) paramAsJarvisParamResult(name string, t abi.Type, value interface{}, hint *ERC20Info) ParamResult {
	result := ParamResult{
		Name: name,
	}

	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		result.Type = t.String()

		realVal, ok := value.(reflect.Value)
		if !ok {
			realVal = reflect.ValueOf(value)
		}

		if t.Elem.T == abi.SliceTy || t.Elem.T == abi.ArrayTy {
			result.Arrays = []ParamResult{}
			for i := 0; i < realVal.Len(); i++ {
				result.Arrays = append(
					result.Arrays,
					ta.paramAsJarvisParamResult(fmt.Sprintf("%s[%d]", name, i), *t.Elem, realVal.Index(i).Interface(), hint))
			}
		} else if t.Elem.T == abi.TupleTy {
			result.Tuples = []TupleParamResult{}
			for i := 0; i < realVal.Len(); i++ {
				result.Tuples = append(
					result.Tuples,
					ta.paramAsJarvisTuple(*t.Elem, realVal.Index(i).Interface(), hint))
			}
		} else {
			result.Values = []Value{}
			for i := 0; i < realVal.Len(); i++ {
				result.Values = append(
					result.Values,
					ta.nonArrayParamAsJarvisValue(*t.Elem, realVal.Index(i).Interface(), hint))
			}
		}
		return result
	case abi.TupleTy:
		result.Type = t.TupleRawName
		result.Tuples = []TupleParamResult{ta.paramAsJarvisTuple(t, value, hint)}
	default:
		result.Type = t.String()
		result.Values = []Value{ta.nonArrayParamAsJarvisValue(t, value, hint)}
	}
	return result
}

// ParamAsJarvisParamResult is the public interface method; it delegates to the
// internal variant with no token hint (nil context).
func (ta *TxAnalyzer) ParamAsJarvisParamResult(name string, t abi.Type, value interface{}) ParamResult {
	return ta.paramAsJarvisParamResult(name, t, value, nil)
}

// ParamAsJarvisParamResultFor is ParamAsJarvisParamResult with the token
// context of contract: when contract is an ERC-20, integer values are
// annotated with its decimals and symbol so "1000000" echoes as "1 USDC".
func (ta *TxAnalyzer) ParamAsJarvisParamResultFor(contract string, name string, t abi.Type, value interface{}) ParamResult {
	var hint *ERC20Info
	if contract != "" {
		hint = ta.ctx.ERC20InfoFor(contract)
	}
	return ta.paramAsJarvisParamResult(name, t, value, hint)
}

// rawTopics turns a log's topics into unnamed TopicResults, topic0 first, for
// logs that no available ABI describes.
func rawTopics(l *types.Log) []TopicResult {
	out := make([]TopicResult, 0, len(l.Topics))
	for i, t := range l.Topics {
		out = append(out, TopicResult{
			Name:  fmt.Sprintf("topic%d", i),
			Value: Value{Raw: t.Hex(), Kind: DisplayRaw},
		})
	}
	return out
}

func findEventById(a *abi.ABI, topic []byte) (*abi.Event, error) {
	for _, event := range a.Events {
		if bytes.Equal(event.ID.Bytes(), topic) {
			return &event, nil
		}
	}
	return nil, fmt.Errorf("no event with id: %#x", topic)
}

// maxRecursionDepth caps AnalyzeFunctionCallRecursively to prevent unbounded
// recursion on crafted or deeply-nested calldata.
const maxRecursionDepth = 10

// LooksLikeTxData returns true only when the params contain exactly one
// "address", exactly one "uint256", and exactly one "bytes" param — matching
// the Gnosis classic submitTransaction(address,uint256,bytes) signature.
func LooksLikeTxData(params []ParamResult) bool {
	var nAddress, nUint, nBytes int
	for _, p := range params {
		switch p.Type {
		case "address":
			nAddress++
		case "uint256":
			nUint++
		case "bytes":
			nBytes++
		}
	}
	return nAddress == 1 && nUint == 1 && nBytes == 1
}

// GetTxDatasFromFunctionCallParams extracts the single address, uint256, and
// bytes values from params that LooksLikeTxData validated.
func GetTxDatasFromFunctionCallParams(
	params []ParamResult,
) (destinations []string, values []string, data []string) {
	for _, p := range params {
		switch p.Type {
		case "address":
			destinations = append(destinations, p.Values[0].Raw)
		case "bytes":
			data = append(data, p.Values[0].Raw)
		case "uint256":
			values = append(values, p.Values[0].Raw)
		}
	}
	return
}

func (self *TxAnalyzer) AnalyzeFunctionCallRecursively(
	lookupABI ABIDatabase,
	value *big.Int,
	destination string,
	data []byte,
	customABIs map[string]*abi.ABI,
) (fc *FunctionCall) {
	return self.analyzeFunctionCallRecursively(lookupABI, value, destination, data, customABIs, 0)
}

func (self *TxAnalyzer) analyzeFunctionCallRecursively(
	lookupABI ABIDatabase,
	value *big.Int,
	destination string,
	data []byte,
	customABIs map[string]*abi.ABI,
	depth int,
) (fc *FunctionCall) {
	fc = &FunctionCall{}
	fc.Destination = self.ctx.GetJarvisAddress(destination)
	fc.Value = value
	fc.Data = data

	var err error

	a := customABIs[strings.ToLower(fc.Destination.Address)]
	if a == nil {
		a, err = lookupABI(destination, self.ctx.Network)
		if err != nil {
			a = GetERC20ABI()
		}
	} else if len(data) >= 4 && !abiHasSelector(a, data[:4]) {
		// A custom ABI that doesn't cover this selector is a partial ABI, not
		// the wrong contract: --abi may name one method, and a Safe batch
		// carries one synthesised method per entry. Fall back to the resolved
		// ABI so an entry the custom one can't describe still decodes; keep
		// the custom ABI when the database has nothing better.
		if resolved, resolveErr := lookupABI(destination, self.ctx.Network); resolveErr == nil &&
			abiHasSelector(resolved, data[:4]) {
			a = resolved
		}
	}

	// Safe batches are delegatecalls into MultiSend / MultiSendCallOnly. Those
	// libraries are often unverified on block explorers, so without this the
	// whole batch would render as one undecoded blob — the operator would be
	// asked to sign an opaque payload.
	if IsMultiSendCallData(data) && !abiHasSelector(a, data[:4]) {
		a = GetMultiSendABI()
	}
	// Classic Gnosis wallets are often unverified proxies. Jarvis already
	// packed confirmTransaction with the built-in ABI; without this the
	// following EOA signing card would render as raw bytes.
	if util.IsGnosisMsigCallData(data) && !abiHasSelector(a, data[:4]) {
		a = util.GetGnosisMsigABI()
	}

	// Look up ERC20 context for the destination so that integer params
	// (token amounts) can be annotated with decimal and symbol.
	hint := self.ctx.ERC20InfoFor(destination)

	if (fc.Destination.Desc == "unknown" || fc.Destination.Desc == "") && hint != nil && hint.Symbol != "" {
		fc.Destination.Desc = hint.Symbol + " token"
		fc.Destination.Decimal = int64(hint.Decimal)
	}

	if len(data) == 0 {
		// A plain value transfer into a contract (receive/fallback): there is
		// no calldata to decode and nothing to report as an error.
		return fc
	}
	fc.Method, fc.Params, err = self.analyzeMethodCall(a, data, hint)
	if err != nil {
		// Keep the underlying reason: "no method with id: 0x..." tells the
		// operator the ABI is missing/wrong, which a generic message doesn't.
		fc.Error = fmt.Sprintf("couldn't decode calldata: %s", err)
	}

	if depth >= maxRecursionDepth {
		return fc
	}

	// A MultiSend batch carries its sub-calls in a hand-packed blob that no
	// amount of ABI decoding will expand, so unpack it explicitly and recurse.
	// This is what lets `msig init`, `msig info` and `msig approve` show every
	// call an owner is actually signing off on rather than one hex argument.
	if IsMultiSendCallData(data) {
		self.appendMultiSendCalls(fc, lookupABI, data, customABIs, depth)
		return fc
	}

	if LooksLikeTxData(fc.Params) {
		destinations, valueStrs, dataStrs := GetTxDatasFromFunctionCallParams(fc.Params)
		n := len(dataStrs)
		if len(destinations) < n {
			n = len(destinations)
		}
		if len(valueStrs) < n {
			n = len(valueStrs)
		}
		for i := 0; i < n; i++ {
			innerData, err := hexutil.Decode(dataStrs[i])
			if err != nil {
				nextFc := &FunctionCall{}
				nextFc.Destination = self.ctx.GetJarvisAddress(destinations[i])
				nextFc.Value = StringToBig(valueStrs[i])
				nextFc.Error = fmt.Sprintf("couldn't decode inner calldata: %s", err)
				fc.DecodedFunctionCalls = append(fc.DecodedFunctionCalls, nextFc)
				continue
			}
			nextFc := self.analyzeFunctionCallRecursively(
				lookupABI,
				StringToBig(valueStrs[i]),
				destinations[i],
				innerData,
				customABIs,
				depth+1,
			)
			fc.DecodedFunctionCalls = append(fc.DecodedFunctionCalls, nextFc)
		}
	}

	return fc
}

// abiHasSelector reports whether a defines a method with the given 4-byte
// selector.
func abiHasSelector(a *abi.ABI, selector []byte) bool {
	if a == nil {
		return false
	}
	_, err := a.MethodById(selector)
	return err == nil
}

// appendMultiSendCalls expands a multiSend(bytes) call into one child
// FunctionCall per batched sub-call. Decode failures are surfaced as child
// entries carrying an Error rather than dropped, so a payload jarvis can't
// read never looks like a payload with nothing in it.
func (self *TxAnalyzer) appendMultiSendCalls(
	fc *FunctionCall,
	lookupABI ABIDatabase,
	data []byte,
	customABIs map[string]*abi.ABI,
	depth int,
) {
	payload, err := unpackMultiSendPayload(data)
	if err != nil {
		fc.DecodedFunctionCalls = append(fc.DecodedFunctionCalls, &FunctionCall{
			Error: fmt.Sprintf("couldn't unpack multiSend argument: %s", err),
		})
		return
	}
	calls, err := DecodeMultiSendPayload(payload)
	if err != nil {
		fc.DecodedFunctionCalls = append(fc.DecodedFunctionCalls, &FunctionCall{
			Error: fmt.Sprintf("couldn't decode multiSend batch: %s", err),
		})
		return
	}

	for _, c := range calls {
		inner := self.analyzeFunctionCallRecursively(
			lookupABI,
			c.Value,
			c.To.Hex(),
			c.Data,
			customABIs,
			depth+1,
		)
		// Operation is 0 for every entry MultiSendCallOnly will accept, so
		// anything else is worth shouting about right where it appears.
		if c.Operation != 0 {
			inner.Error = strings.TrimSpace(fmt.Sprintf(
				"%s [batch entry uses operation %d (DELEGATECALL) — DANGEROUS]",
				inner.Error, c.Operation,
			))
		}
		fc.DecodedFunctionCalls = append(fc.DecodedFunctionCalls, inner)
	}
}

// unpackMultiSendPayload pulls the `transactions` argument out of
// multiSend(bytes) calldata.
func unpackMultiSendPayload(data []byte) ([]byte, error) {
	m, ok := GetMultiSendABI().Methods[MultiSendMethodName]
	if !ok {
		return nil, fmt.Errorf("multiSend missing from the built-in ABI")
	}
	values, err := m.Inputs.UnpackValues(data[4:])
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("expected 1 argument, got %d", len(values))
	}
	payload, ok := values[0].([]byte)
	if !ok {
		return nil, fmt.Errorf("expected bytes, got %T", values[0])
	}
	return payload, nil
}

// analyzeMethodCall is the internal variant that accepts a token hint so that
// ERC20 integer params can be annotated with decimal context.
func (self *TxAnalyzer) analyzeMethodCall(
	a *abi.ABI,
	data []byte,
	hint *ERC20Info,
) (method string, params []ParamResult, err error) {
	m, err := a.MethodById(data)
	if err != nil {
		// Unknown selector — fall back to the standard ERC20 ABI.
		a = GetERC20ABI()
		m, err = a.MethodById(data)
	}
	if err != nil {
		return "", []ParamResult{}, err
	}
	// RawName is the Solidity name; Name carries go-ethereum's overload
	// suffix ("execute0"), which means nothing to the operator.
	method = m.RawName
	ps, err := m.Inputs.UnpackValues(data[4:])
	if err != nil {
		return method, []ParamResult{}, err
	}

	params = []ParamResult{}
	for i, input := range m.Inputs {
		params = append(params, self.paramAsJarvisParamResult(input.Name, input.Type, ps[i], hint))
	}

	return method, params, nil
}

// AnalyzeMethodCall is the public interface method; it delegates to the
// internal variant with no token hint (nil context).
func (self *TxAnalyzer) AnalyzeMethodCall(
	a *abi.ABI,
	data []byte,
) (method string, params []ParamResult, err error) {
	return self.analyzeMethodCall(a, data, nil)
}

// isValueType reports whether an ABI type is stored by value in an event topic.
// Reference types (string, bytes, dynamic arrays, tuples) are stored as their
// keccak256 hash instead and cannot be decoded from the raw topic bytes.
func isValueType(t abi.Type) bool {
	switch t.T {
	case abi.BoolTy, abi.UintTy, abi.IntTy, abi.AddressTy, abi.HashTy, abi.FixedBytesTy:
		return true
	default:
		return false
	}
}

// AnalyzeLog decodes one event log. lookupABI resolves the emitting contract's
// ABI when customABIs has no entry for it; pass nil to use the plain util.GetABI
// lookup. Callers that hold a --abi fallback should pass the same ABIDatabase
// they used for the calldata so an unverified contract's events decode too.
func (self *TxAnalyzer) AnalyzeLog(
	lookupABI ABIDatabase,
	customABIs map[string]*abi.ABI,
	l *types.Log,
) (LogResult, error) {
	logResult := LogResult{
		Name:    "",
		Address: self.ctx.GetJarvisAddress(l.Address.Hex()),
		Topics:  []TopicResult{},
		Data:    []ParamResult{},
	}

	var err error

	if len(l.Topics) == 0 {
		return logResult, fmt.Errorf("log from %s has no topics", l.Address.Hex())
	}

	a := customABIs[strings.ToLower(l.Address.Hex())]
	if a == nil {
		if lookupABI == nil {
			lookupABI = util.GetABI
		}
		a, err = lookupABI(l.Address.Hex(), self.ctx.Network)
		if err != nil {
			a = nil
		}
	}
	var event *abi.Event
	if a != nil {
		event, _ = findEventById(a, l.Topics[0].Bytes())
	}
	// Standard token events decode the same on every contract, so an
	// unverified token still contributes to Transfers. The ERC-721 variant is
	// picked by topic count (token id is a third indexed argument).
	if event == nil {
		event, _ = findEventById(GetWellKnownEventsABI(len(l.Topics) == 4), l.Topics[0].Bytes())
	}
	if event == nil {
		// Keep the log in the result so the event count stays honest; the
		// display layer renders it from the raw topics.
		logResult.Topics = rawTopics(l)
		if len(l.Data) > 0 {
			logResult.Data = append(logResult.Data, ParamResult{
				Name:   "data",
				Type:   "bytes",
				Values: []Value{{Raw: hexutil.Encode(l.Data), Kind: DisplayRaw}},
			})
		}
		return logResult, nil
	}
	logResult.Name = event.Name

	// Annotate token amounts if the emitting contract is a known ERC20.
	hint := self.ctx.ERC20InfoFor(l.Address.Hex())
	if hint != nil && hint.Symbol != "" && !IsKnownAddress(logResult.Address) {
		logResult.Address.Desc = hint.Symbol + " token"
		logResult.Address.Decimal = int64(hint.Decimal)
	}

	iArgs, niArgs := SplitEventArguments(event.Inputs)
	for j, topic := range l.Topics[1:] {
		name := fmt.Sprintf("topic%d", j+1)
		var arg abi.Argument
		if j < len(iArgs) {
			arg = iArgs[j]
			name = arg.Name
		}
		var topicValue Value
		if j < len(iArgs) && isValueType(arg.Type) {
			// Value types are ABI-encoded as 32-byte words in the topic slot;
			// we can decode them with full type fidelity.
			singleArg := abi.Arguments{abi.Argument{Name: arg.Name, Type: arg.Type}}
			if decoded, decErr := singleArg.Unpack(topic.Bytes()); decErr == nil && len(decoded) > 0 {
				topicValue = self.nonArrayParamAsJarvisValue(arg.Type, decoded[0], hint)
			} else {
				topicValue = Value{Raw: topic.Hex(), Kind: DisplayRaw}
			}
		} else {
			// Reference types are stored as keccak256 hashes and cannot be decoded.
			topicValue = Value{Raw: topic.Hex(), Kind: DisplayRaw}
		}
		logResult.Topics = append(logResult.Topics, TopicResult{
			Name:  name,
			Value: topicValue,
		})
	}

	params, err := niArgs.UnpackValues(l.Data)
	if err != nil {
		return logResult, err
	}
	for i, input := range niArgs {
		logResult.Data = append(logResult.Data, self.paramAsJarvisParamResult(input.Name, input.Type, params[i], hint))
	}
	return logResult, nil
}

func (self *TxAnalyzer) analyzeContractTx(
	txinfo TxInfo,
	lookupABI ABIDatabase,
	customABIs map[string]*abi.ABI,
	result *TxResult,
) {
	result.FunctionCall = self.AnalyzeFunctionCallRecursively(
		lookupABI,
		txinfo.Tx.Value(),
		txinfo.Tx.To().Hex(),
		txinfo.Tx.Data(),
		customABIs)
	self.analyzeLogs(txinfo, lookupABI, customABIs, result)
}

func (self *TxAnalyzer) analyzeLogs(
	txinfo TxInfo,
	lookupABI ABIDatabase,
	customABIs map[string]*abi.ABI,
	result *TxResult,
) {
	if txinfo.Receipt == nil {
		return
	}
	for _, l := range txinfo.Receipt.Logs {
		logResult, err := self.AnalyzeLog(lookupABI, customABIs, l)
		if err != nil {
			if result.Error != "" {
				result.Error += "; "
			}
			result.Error += err.Error()
			continue
		}
		result.Logs = append(result.Logs, logResult)
	}
}

func (self *TxAnalyzer) AnalyzeOffline(
	txinfo *TxInfo,
	lookupABI ABIDatabase,
	customABIs map[string]*abi.ABI,
	isContract bool,
) *TxResult {
	result := NewTxResult()
	result.Network = self.ctx.Network.GetName()
	result.Hash = txinfo.Tx.Hash().Hex()
	result.Status = txinfo.Status
	if txinfo.Status == "done" || txinfo.Status == "reverted" {
		self.setBasicTxInfo(*txinfo, result)
		if txinfo.Status == "reverted" {
			result.RevertReason = self.revertReason(txinfo, lookupABI, customABIs)
		}
		switch {
		case txinfo.Tx.To() == nil:
			result.TxType = "contract creation"
			self.analyzeLogs(*txinfo, lookupABI, customABIs, result)
		case !isContract:
			result.TxType = "normal"
		default:
			result.TxType = "contract call"
			self.analyzeContractTx(*txinfo, lookupABI, customABIs, result)

			if result.To.Desc == "unknown" || result.To.Desc == "" {
				if hint := self.ctx.ERC20InfoFor(txinfo.Tx.To().Hex()); hint != nil && hint.Symbol != "" {
					result.To.Desc = hint.Symbol + " token"
					result.To.Decimal = int64(hint.Decimal)
				}
			}
		}
	}
	return result
}

// NewGenericAnalyzer creates a TxAnalyzer with the default (production)
// address resolver backed by the local address databases.
func NewGenericAnalyzer(r reader.Reader, network Network) *TxAnalyzer {
	return &TxAnalyzer{ctx: NewAnalysisContext(r, network)}
}

// NewGenericAnalyzerWithContext creates a TxAnalyzer with a fully-configured
// AnalysisContext, allowing callers to inject a custom AddressResolver (e.g.
// addrbook.Map for tests) and any other context-level dependencies.
//
// Typical test usage:
//
//	ctx := txanalyzer.NewAnalysisContextWithResolver(reader, network,
//	    addrbook.Map{"0xabc...": "Alice"})
//	analyzer := txanalyzer.NewGenericAnalyzerWithContext(ctx)
func NewGenericAnalyzerWithContext(ctx *AnalysisContext) *TxAnalyzer {
	return &TxAnalyzer{ctx: ctx}
}

// revertReplayer is implemented by readers that can replay a mined tx to
// recover its revert payload (see reader.EthReader.RevertData).
type revertReplayer interface {
	RevertData(txinfo TxInfo) ([]byte, error)
}

// revertReason replays a reverted tx and decodes the payload. Any failure
// yields "" — the reason is a convenience, never a requirement.
func (self *TxAnalyzer) revertReason(txinfo *TxInfo, lookupABI ABIDatabase, customABIs map[string]*abi.ABI) string {
	rp, ok := self.ctx.reader.(revertReplayer)
	if !ok || txinfo.Tx == nil || txinfo.Tx.To() == nil {
		return ""
	}
	data, err := rp.RevertData(*txinfo)
	if err != nil || len(data) == 0 {
		return ""
	}
	var a *abi.ABI
	if sel := hexutil.Encode(data[:min(4, len(data))]); sel != errorStringSelector && sel != panicSelector {
		to := txinfo.Tx.To().Hex()
		a = customABIs[strings.ToLower(to)]
		if a == nil && lookupABI != nil {
			a, _ = lookupABI(to, self.ctx.Network)
		}
	}
	return DecodeRevertData(data, a)
}

const (
	errorStringSelector = "0x08c379a0" // Error(string)
	panicSelector       = "0x4e487b71" // Panic(uint256)
)

var panicCodes = map[uint64]string{
	0x00: "generic compiler panic",
	0x01: "assert failed",
	0x11: "arithmetic overflow or underflow",
	0x12: "division or modulo by zero",
	0x21: "invalid enum value",
	0x22: "corrupted storage byte array",
	0x31: "pop on empty array",
	0x32: "array index out of bounds",
	0x41: "out of memory",
	0x51: "call to uninitialised function pointer",
}

// DecodeRevertData renders a revert payload for humans: Error(string) as the
// quoted message, Panic(uint256) as its meaning, a custom error by name when a
// carries its definition, and the bare selector otherwise.
func DecodeRevertData(data []byte, a *abi.ABI) string {
	if len(data) < 4 {
		if len(data) == 0 {
			return ""
		}
		return fmt.Sprintf("revert data %s", hexutil.Encode(data))
	}
	selector, payload := data[:4], data[4:]
	stringT, _ := abi.NewType("string", "", nil)
	uintT, _ := abi.NewType("uint256", "", nil)
	switch hexutil.Encode(selector) {
	case errorStringSelector:
		if vals, err := (abi.Arguments{{Type: stringT}}).Unpack(payload); err == nil && len(vals) == 1 {
			return fmt.Sprintf("%q", vals[0])
		}
	case panicSelector:
		if vals, err := (abi.Arguments{{Type: uintT}}).Unpack(payload); err == nil && len(vals) == 1 {
			code := vals[0].(*big.Int).Uint64()
			if msg, ok := panicCodes[code]; ok {
				return fmt.Sprintf("panic 0x%02x: %s", code, msg)
			}
			return fmt.Sprintf("panic 0x%02x", code)
		}
	}
	if a != nil {
		for _, e := range a.Errors {
			if !bytes.Equal(e.ID.Bytes()[:4], selector) {
				continue
			}
			vals, err := e.Inputs.Unpack(payload)
			if err != nil {
				return e.Name + "(…)"
			}
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = fmt.Sprintf("%v", v)
			}
			return fmt.Sprintf("%s(%s)", e.Name, strings.Join(parts, ", "))
		}
	}
	return fmt.Sprintf("custom error %s", hexutil.Encode(selector))
}
