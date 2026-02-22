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

func (self *TxAnalyzer) setBasicTxInfo(txinfo TxInfo, result *TxResult, network Network) {
	result.From = util.GetJarvisAddress(txinfo.Tx.Extra.From.Hex(), self.ctx.Network)
	result.Value = fmt.Sprintf(
		"%s",
		BigToFloatString(txinfo.Tx.Value(), network.GetNativeTokenDecimal()),
	)
	result.To = util.GetJarvisAddress(txinfo.Tx.To().Hex(), self.ctx.Network)
	result.Nonce = fmt.Sprintf("%d", txinfo.Tx.Nonce())
	result.GasPrice = fmt.Sprintf("%.4f", BigToFloat(txinfo.Tx.GasPrice(), 9))
	result.GasLimit = fmt.Sprintf("%d", txinfo.Tx.Gas())
	result.GasUsed = fmt.Sprintf("%d", txinfo.Receipt.GasUsed)
	result.GasCost = fmt.Sprintf(
		"%.8f",
		BigToFloat(txinfo.GasCost(), network.GetNativeTokenDecimal()),
	)
}

// nonArrayParamAsJarvisValue converts a scalar ABI value to a jarvis Value.
// When hint is non-nil (i.e. the contract is a known ERC20), integer values
// are annotated with the token's decimal and symbol so the display layer can
// show both the raw amount and the human-readable form.
func (self *TxAnalyzer) nonArrayParamAsJarvisValue(t abi.Type, value interface{}, hint *ERC20Info) Value {
	valueStr := ""
	switch t.T {
	case abi.StringTy:
		valueStr = fmt.Sprintf("%s", value.(string))
	case abi.IntTy, abi.UintTy:
		valueStr = fmt.Sprintf("%d", value)
		v := util.GetJarvisValue(valueStr, self.ctx.Network)
		if hint != nil {
			v.TokenHint = &TokenHint{
				Decimal: hint.Decimal,
				Symbol:  hint.Symbol,
			}
		}
		return v
	case abi.BoolTy:
		valueStr = fmt.Sprintf("%t", value.(bool))
	case abi.AddressTy:
		valueStr = fmt.Sprintf("%s", value.(common.Address).Hex())
	case abi.HashTy:
		valueStr = fmt.Sprintf("%s", value.(common.Hash).Hex())
	case abi.BytesTy:
		valueStr = fmt.Sprintf("0x%s", common.Bytes2Hex(value.([]byte)))
	case abi.FixedBytesTy:
		word := []byte{}
		for i := 0; i < int(reflect.TypeOf(value).Size()); i++ {
			word = append(word, byte(0))
		}
		reflect.Copy(reflect.ValueOf(word), reflect.ValueOf(value))
		valueStr = fmt.Sprintf("0x%s", common.Bytes2Hex(word))
	case abi.FunctionTy:
		valueStr = fmt.Sprintf("0x%s", common.Bytes2Hex(value.([]byte)))
	default:
		valueStr = fmt.Sprintf("%v", value)
	}
	return util.GetJarvisValue(valueStr, self.ctx.Network)
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
			destinations = append(destinations, p.Values[0].Value)
		case "bytes":
			data = append(data, p.Values[0].Value)
		case "uint256":
			values = append(values, p.Values[0].Value)
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
	fc.Destination = util.GetJarvisAddress(destination, self.ctx.Network)
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
	if _, err := a.MethodById(data); err != nil {
		a = GetERC20ABI()
	}
	m, err := a.MethodById(data)
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
		logResult.Topics = append(logResult.Topics, TopicResult{
			Name:  iArgs[j].Name,
			Value: []Value{util.GetJarvisValue(topic.Hex(), self.ctx.Network)},
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
	network Network,
) {
	result.FunctionCall = self.AnalyzeFunctionCallRecursively(
		lookupABI,
		txinfo.Tx.Value(),
		txinfo.Tx.To().Hex(),
		txinfo.Tx.Data(),
		customABIs)

	logs := txinfo.Receipt.Logs
	for _, l := range logs {
		logResult, err := self.AnalyzeLog(customABIs, l)
		if err != nil {
			result.Error += fmt.Sprintf("%s", err)
		}
		result.Logs = append(result.Logs, logResult)
	}
}

func (self *TxAnalyzer) AnalyzeOffline(
	txinfo *TxInfo,
	lookupABI ABIDatabase,
	customABIs map[string]*abi.ABI,
	isContract bool,
	network Network,
) *TxResult {
	result := NewTxResult()
	result.Network = self.ctx.Network.GetName()
	result.Hash = txinfo.Tx.Hash().Hex()
	result.Status = txinfo.Status
	if txinfo.Status == "done" || txinfo.Status == "reverted" {
		self.setBasicTxInfo(*txinfo, result, network)
		if !isContract {
			result.TxType = "normal"
		} else {
			result.TxType = "contract call"
			self.analyzeContractTx(*txinfo, lookupABI, customABIs, result, network)
		}
	}
	return result
}

func NewGenericAnalyzer(r reader.Reader, network Network) *TxAnalyzer {
	return &TxAnalyzer{ctx: NewAnalysisContext(r, network)}
}
