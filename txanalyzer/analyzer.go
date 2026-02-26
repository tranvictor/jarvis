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

func EthAnalyzer(network Network) (*TxAnalyzer, error) {
	r, err := util.EthReader(network)
	if err != nil {
		return nil, err
	}
	return NewGenericAnalyzer(r, network), nil
}

type TxAnalyzer struct {
	ctx *AnalysisContext
}

func (self *TxAnalyzer) setBasicTxInfo(txinfo TxInfo, result *TxResult) {
	result.From = self.ctx.GetJarvisAddress(txinfo.Tx.Extra.From.Hex())
	result.Value = BigToFloatString(txinfo.Tx.Value(), self.ctx.Network.GetNativeTokenDecimal())
	result.To = self.ctx.GetJarvisAddress(txinfo.Tx.To().Hex())
	result.Nonce = fmt.Sprintf("%d", txinfo.Tx.Nonce())
	result.GasPrice = fmt.Sprintf("%.4f", BigToFloat(txinfo.Tx.GasPrice(), 9))
	result.GasLimit = fmt.Sprintf("%d", txinfo.Tx.Gas())
	result.GasUsed = fmt.Sprintf("%d", txinfo.Receipt.GasUsed)
	result.GasCost = fmt.Sprintf("%.8f", BigToFloat(txinfo.GasCost(), self.ctx.Network.GetNativeTokenDecimal()))
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

	var err error

	a := customABIs[strings.ToLower(fc.Destination.Address)]
	if a == nil {
		a, err = lookupABI(destination, self.ctx.Network)
		if err != nil {
			a = GetERC20ABI()
		}
	}

	// Look up ERC20 context for the destination so that integer params
	// (token amounts) can be annotated with decimal and symbol.
	hint := self.ctx.ERC20InfoFor(destination)

	fc.Method, fc.Params, err = self.analyzeMethodCall(a, data, hint)
	if err != nil {
		fc.Error = "couldn't decode bytes data"
	}

	if depth >= maxRecursionDepth {
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
	method = m.Name
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

func (self *TxAnalyzer) AnalyzeLog(
	customABIs map[string]*abi.ABI,
	l *types.Log,
) (LogResult, error) {
	logResult := LogResult{
		Name:   "",
		Topics: []TopicResult{},
		Data:   []ParamResult{},
	}

	var err error

	a := customABIs[strings.ToLower(l.Address.Hex())]
	if a == nil {
		a, err = util.GetABI(l.Address.Hex(), self.ctx.Network)
		if err != nil {
			return logResult, fmt.Errorf("getting abi for %s failed: %s", l.Address.Hex(), err)
		}
	}
	event, err := findEventById(a, l.Topics[0].Bytes())
	if err != nil {
		return logResult, err
	}
	logResult.Name = event.Name

	// Annotate token amounts if the emitting contract is a known ERC20.
	hint := self.ctx.ERC20InfoFor(l.Address.Hex())

	iArgs, niArgs := SplitEventArguments(event.Inputs)
	for j, topic := range l.Topics[1:] {
		arg := iArgs[j]
		var topicValue Value
		if isValueType(arg.Type) {
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
			Name:  arg.Name,
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

	for _, l := range txinfo.Receipt.Logs {
		logResult, err := self.AnalyzeLog(customABIs, l)
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
		if !isContract {
			result.TxType = "normal"
		} else {
			result.TxType = "contract call"
			self.analyzeContractTx(*txinfo, lookupABI, customABIs, result)
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
