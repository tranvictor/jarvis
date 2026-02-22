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
	reader  reader.Reader
	Network Network
}

func (self *TxAnalyzer) setBasicTxInfo(txinfo TxInfo, result *TxResult, network Network) {
	result.From = util.GetJarvisAddress(txinfo.Tx.Extra.From.Hex(), self.Network)
	result.Value = fmt.Sprintf(
		"%s",
		BigToFloatString(txinfo.Tx.Value(), network.GetNativeTokenDecimal()),
	)
	result.To = util.GetJarvisAddress(txinfo.Tx.To().Hex(), self.Network)
	result.Nonce = fmt.Sprintf("%d", txinfo.Tx.Nonce())
	result.GasPrice = fmt.Sprintf("%.4f", BigToFloat(txinfo.Tx.GasPrice(), 9))
	result.GasLimit = fmt.Sprintf("%d", txinfo.Tx.Gas())
	result.GasUsed = fmt.Sprintf("%d", txinfo.Receipt.GasUsed)
	result.GasCost = fmt.Sprintf(
		"%.8f",
		BigToFloat(txinfo.GasCost(), network.GetNativeTokenDecimal()),
	)
	// result.Timestamp = fmt.Sprintf("%s", time.Unix(int64(txinfo.BlockHeader.Time), 0).String())
}

func (self *TxAnalyzer) nonArrayParamAsJarvisValue(t abi.Type, value interface{}) Value {
	valueStr := ""
	switch t.T {
	case abi.StringTy: // variable arrays are written at the end of the return bytes
		valueStr = fmt.Sprintf("%s", value.(string))
	case abi.IntTy, abi.UintTy:
		valueStr = fmt.Sprintf("%d", value)
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
	return util.GetJarvisValue(valueStr, self.Network)
}

func (ta *TxAnalyzer) ParamAsJarvisTuple(t abi.Type, value interface{}) TupleParamResult {
	result := TupleParamResult{
		Name: t.TupleRawName,
		Type: t.String(),
	}

	realVal, ok := value.(reflect.Value)
	if !ok {
		// value is not a reflect.Value
		realVal = reflect.ValueOf(value)
	}

	for i, field := range t.TupleElems {
		result.Values = append(result.Values, ta.ParamAsJarvisParamResult(
			t.TupleRawNames[i],
			*field,
			reflect.Indirect(realVal).FieldByName(
				cases.Title(language.Und, cases.NoLower).String(t.TupleRawNames[i]),
			).Interface(),
		))
	}
	return result
}

func (ta *TxAnalyzer) ParamAsJarvisParamResult(name string, t abi.Type, value interface{}) ParamResult {
	result := ParamResult{
		Name: name,
	}

	switch t.T {
	case abi.SliceTy, abi.ArrayTy:
		result.Type = t.String()

		realVal, ok := value.(reflect.Value)
		if !ok {
			// value is not a reflect.Value
			realVal = reflect.ValueOf(value)
		}

		// check to see if element of this argument is either slice, array, tuple or arbitrary types
		if t.Elem.T == abi.SliceTy || t.Elem.T == abi.ArrayTy {
			// if the element is a slice, array or tuple, we need to populate the result's tuples
			result.Arrays = []ParamResult{}
			for i := 0; i < realVal.Len(); i++ {
				result.Arrays = append(
					result.Arrays,
					ta.ParamAsJarvisParamResult(fmt.Sprintf("%s[%d]", name, i), *t.Elem, realVal.Index(i).Interface()))
			}
		} else if t.Elem.T == abi.TupleTy {
			result.Tuples = []TupleParamResult{}
			for i := 0; i < realVal.Len(); i++ {
				result.Tuples = append(
					result.Tuples,
					ta.ParamAsJarvisTuple(*t.Elem, realVal.Index(i).Interface()))
			}
		} else {
			result.Values = []Value{}
			for i := 0; i < realVal.Len(); i++ {
				result.Values = append(
					result.Values,
					ta.nonArrayParamAsJarvisValue(*t.Elem, realVal.Index(i).Interface()))
			}
		}
		return result
	case abi.TupleTy:
		result.Type = t.TupleRawName
		result.Tuples = []TupleParamResult{ta.ParamAsJarvisTuple(t, value)}
	default:
		result.Type = t.String()
		result.Values = []Value{ta.nonArrayParamAsJarvisValue(t, value)}
	}
	return result
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
// The previous implementation matched any function that happened to have at
// least one of each type, causing false-positive recursive decoding.
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
// bytes values from params that LooksLikeTxData validated. It returns exactly
// one element in each slice so the caller's loop is always safe.
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
	fc.Destination = util.GetJarvisAddress(destination, self.Network)
	fc.Value = value

	var err error

	a := customABIs[strings.ToLower(fc.Destination.Address)]
	if a == nil {
		a, err = lookupABI(destination, self.Network)
		if err != nil {
			a = GetERC20ABI()
		}
	}

	fc.Method, fc.Params, err = self.AnalyzeMethodCall(a, data)
	if err != nil {
		fc.Error = "couldn't decode bytes data"
	}

	if depth >= maxRecursionDepth {
		return fc
	}

	if LooksLikeTxData(fc.Params) {
		destinations, valueStrs, dataStrs := GetTxDatasFromFunctionCallParams(fc.Params)
		// All three slices have exactly one element (guaranteed by LooksLikeTxData
		// + GetTxDatasFromFunctionCallParams), so iterating any one is safe.
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

func (self *TxAnalyzer) AnalyzeMethodCall(
	a *abi.ABI,
	data []byte,
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
		params = append(params, self.ParamAsJarvisParamResult(input.Name, input.Type, ps[i]))
	}

	return method, params, nil
}

func (self *TxAnalyzer) AnalyzeLog(
	customABIs map[string]*abi.ABI,
	l *types.Log,
	network Network,
) (LogResult, error) {
	logResult := LogResult{
		Name:   "",
		Topics: []TopicResult{},
		Data:   []ParamResult{},
	}

	var err error

	abi := customABIs[strings.ToLower(l.Address.Hex())]
	if abi == nil {
		abi, err = util.GetABI(l.Address.Hex(), network)
		if err != nil {
			return logResult, fmt.Errorf("getting abi for %s failed: %s", l.Address.Hex(), err)
		}
	}
	event, err := findEventById(abi, l.Topics[0].Bytes())
	if err != nil {
		return logResult, err
	}
	logResult.Name = event.Name

	iArgs, niArgs := SplitEventArguments(event.Inputs)
	for j, topic := range l.Topics[1:] {
		logResult.Topics = append(logResult.Topics, TopicResult{
			Name:  iArgs[j].Name,
			Value: []Value{util.GetJarvisValue(topic.Hex(), self.Network)},
		})
	}

	params, err := niArgs.UnpackValues(l.Data)
	if err != nil {
		return logResult, err
	}
	for i, input := range niArgs {
		logResult.Data = append(logResult.Data, self.ParamAsJarvisParamResult(input.Name, input.Type, params[i]))
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
		logResult, err := self.AnalyzeLog(customABIs, l, network)
		if err != nil {
			result.Error += fmt.Sprintf("%s", err)
		}
		result.Logs = append(result.Logs, logResult)
	}
}

// this function only compares thee function and param names against
// standard gnosis multisig. No bytecode is compared.
// func isGnosisMultisig(method *abi.Method, params []interface{}) bool {
// 	if method.Name != "submitTransaction" {
// 		return false
// 	}
// 	expectedParams := []string{"destination", "value", "data"}
// 	for i, input := range method.Inputs {
// 		if input.Name != expectedParams[i] {
// 			return false
// 		}
// 	}
// 	return true
// }
//
// func (self *TxAnalyzer) gnosisMultisigInitData(a *abi.ABI, params []interface{}, customABIs map[string]*abi.ABI) (result *GnosisResult) {
// 	result = &GnosisResult{
// 		Contract:   Address{},
// 		Network:    self.Network,
// 		Method:     "",
// 		Params:     []ParamResult{},
// 		GnosisInit: nil,
// 		Error:      "",
// 	}
// 	var err error
// 	contract := params[0].(common.Address)
// 	result.Contract = util.GetJarvisAddress(contract.Hex(), self.Network)
// 	data := params[2].([]byte)
// 	if a == nil {
// 		result.Error = fmt.Sprintf("Cannot get abi of the contract: %s", err)
// 		return result
// 	}
// 	if _, err = a.MethodById(data); err != nil {
// 		a, _ = GetERC20ABI()
// 	}
// 	method, err := a.MethodById(data)
// 	if err != nil {
// 		result.Error = fmt.Sprintf("Cannot get corresponding method from the ABI: %s", err)
// 		return result
// 	}
// 	// fmt.Printf("    Method: %s\n", method.Name)
// 	result.Method = method.Name
// 	ps, err := method.Inputs.UnpackValues(data[4:])
// 	if err != nil {
// 		result.Error = fmt.Sprintf("Cannot parse params: %s", err)
// 		return result
// 	}
// 	// fmt.Printf("    Params:\n")
// 	for i, input := range method.Inputs {
// 		result.Params = append(result.Params, ParamResult{
// 			Name:  input.Name,
// 			Type:  input.Type.String(),
// 			Value: self.ParamAsJarvisValues(input.Type, ps[i]),
// 		})
// 		// fmt.Printf("        %s (%s): ", input.Name, input.Type)
// 	}
//
// 	if isGnosisMultisig(method, ps) {
// 		// fmt.Printf("    ==> Gnosis Multisig init data:\n")
// 		contract := ps[0].(common.Address)
// 		a := customABIs[strings.ToLower(contract.Hex())]
// 		if a == nil {
// 			a, err = util.GetABI(contract.Hex(), self.Network)
// 			if err != nil {
// 				a, _ = GetERC20ABI()
// 			}
// 		}
// 		result.GnosisInit = self.gnosisMultisigInitData(a, ps, customABIs)
// 	}
// 	return result
// }

func (self *TxAnalyzer) AnalyzeOffline(
	txinfo *TxInfo,
	lookupABI ABIDatabase,
	customABIs map[string]*abi.ABI,
	isContract bool,
	network Network,
) *TxResult {
	result := NewTxResult()
	result.Network = self.Network.GetName()
	// fmt.Printf("==========================================Transaction info===============================================================\n")
	// fmt.Printf("tx hash: %s\n", tx)
	result.Hash = txinfo.Tx.Hash().Hex()
	// fmt.Printf("mining status: %s\n", txinfo.Status)
	result.Status = txinfo.Status
	if txinfo.Status == "done" || txinfo.Status == "reverted" {
		self.setBasicTxInfo(*txinfo, result, network)
		if !isContract {
			// fmt.Printf("tx type: normal\n")
			result.TxType = "normal"
		} else {
			// fmt.Printf("tx type: contract call\n")
			result.TxType = "contract call"
			self.analyzeContractTx(*txinfo, lookupABI, customABIs, result, network)
		}
	}
	// fmt.Printf("=========================================================================================================================\n")
	return result
}

func NewGenericAnalyzer(r reader.Reader, network Network) *TxAnalyzer {
	return &TxAnalyzer{r, network}
}
