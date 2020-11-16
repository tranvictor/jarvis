package txanalyzer

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/tranvictor/ethutils"
	"github.com/tranvictor/ethutils/reader"
)

type TxAnalyzer struct {
	reader  *reader.EthReader
	Network string
}

func (self *TxAnalyzer) setBasicTxInfo(txinfo ethutils.TxInfo, result *TxResult) {
	result.From = txinfo.Tx.Extra.From.Hex()
	result.Value = fmt.Sprintf("%f", ethutils.BigToFloat(txinfo.Tx.Value(), 18))
	result.To = txinfo.Tx.To().Hex()
	result.Nonce = fmt.Sprintf("%d", txinfo.Tx.Nonce())
	result.GasPrice = fmt.Sprintf("%f", ethutils.BigToFloat(txinfo.Tx.GasPrice(), 9))
	result.GasLimit = fmt.Sprintf("%d", txinfo.Tx.Gas())
}

func (self *TxAnalyzer) nonArrayParamAsString(t abi.Type, value interface{}) string {
	switch t.T {
	case abi.StringTy: // variable arrays are written at the end of the return bytes
		return fmt.Sprintf("%s", value.(string))
	case abi.IntTy, abi.UintTy:
		return fmt.Sprintf("%d", value)
	case abi.BoolTy:
		return fmt.Sprintf("%t", value.(bool))
	case abi.AddressTy:
		return fmt.Sprintf("%s", value.(common.Address).Hex())
	case abi.HashTy:
		return fmt.Sprintf("%s", value.(common.Hash).Hex())
	case abi.BytesTy:
		return fmt.Sprintf("0x%s", common.Bytes2Hex(value.([]byte)))
	case abi.FixedBytesTy:
		word := []byte{}
		for i := 0; i < int(reflect.TypeOf(value).Size()); i++ {
			word = append(word, byte(0))
		}
		reflect.Copy(reflect.ValueOf(word), reflect.ValueOf(value))
		return fmt.Sprintf("0x%s", common.Bytes2Hex(word))
	case abi.FunctionTy:
		return fmt.Sprintf("0x%s", common.Bytes2Hex(value.([]byte)))
	default:
		return fmt.Sprintf("%v", value)
	}
}

func (self *TxAnalyzer) ParamAsStrings(t abi.Type, value interface{}) []string {
	switch t.T {
	case abi.SliceTy:
		realVal := reflect.ValueOf(value)
		result := []string{}
		for i := 0; i < realVal.Len(); i++ {
			result = append(result, self.ParamAsStrings(*t.Elem, realVal.Index(i).Interface())...)
		}
		return result
	case abi.ArrayTy:
		realVal := reflect.ValueOf(value)
		result := []string{}
		for i := 0; i < realVal.Len(); i++ {
			result = append(result, self.ParamAsStrings(*t.Elem, realVal.Index(i).Interface())...)
		}
		return result
	default:
		return []string{self.nonArrayParamAsString(t, value)}
	}
}

func findEventById(a *abi.ABI, topic []byte) (*abi.Event, error) {
	for _, event := range a.Events {
		if bytes.Equal(event.ID.Bytes(), topic) {
			return &event, nil
		}
	}
	return nil, fmt.Errorf("no event with id: %#x", topic)
}

func (self *TxAnalyzer) AnalyzeMethodCall(abi *abi.ABI, data []byte) (method string, params []ParamResult, gnosisResult *GnosisResult, err error) {
	if _, err := abi.MethodById(data); err != nil {
		abi, _ = ethutils.GetERC20ABI()
	}
	m, err := abi.MethodById(data)
	if err != nil {
		return "", []ParamResult{}, nil, err
	}
	method = m.Name
	ps, err := m.Inputs.UnpackValues(data[4:])
	if err != nil {
		return method, []ParamResult{}, nil, err
	}
	params = []ParamResult{}
	for i, input := range m.Inputs {
		params = append(params, ParamResult{
			Name:  input.Name,
			Type:  input.Type.String(),
			Value: self.ParamAsStrings(input.Type, ps[i]),
		})
	}

	if isGnosisMultisig(m, ps) {
		// fmt.Printf("    ==> Gnosis Multisig init data:\n")
		contract := ps[0].(common.Address)
		a, err := self.reader.GetABI(contract.Hex())
		if err != nil {
			a, _ = ethutils.GetERC20ABI()
		}
		gnosisResult = self.gnosisMultisigInitData(a, ps)
	}
	return method, params, gnosisResult, nil
}

func (self *TxAnalyzer) AnalyzeLog(abi *abi.ABI, l *types.Log) (LogResult, error) {
	logResult := LogResult{
		Name:   "",
		Topics: []TopicResult{},
		Data:   []ParamResult{},
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
			Value: []string{topic.Hex()},
		})
	}

	params, err := niArgs.UnpackValues(l.Data)
	if err != nil {
		return logResult, err
	}
	for i, input := range niArgs {
		logResult.Data = append(logResult.Data, ParamResult{
			Name:  input.Name,
			Type:  input.Type.String(),
			Value: self.ParamAsStrings(input.Type, params[i]),
		})
	}
	return logResult, nil
}

func (self *TxAnalyzer) analyzeContractTx(txinfo ethutils.TxInfo, abi *abi.ABI, result *TxResult) {
	result.Contract = txinfo.Tx.To().Hex()
	// fmt.Printf("------------------------------------------Contract call info-------------------------------------------------------------\n")
	data := txinfo.Tx.Data()
	methodName, params, gnosisResult, err := self.AnalyzeMethodCall(abi, data)
	if err != nil {
		result.Error = fmt.Sprintf("Cannot analyze the method call: %s", err)
		return
	}

	result.Method = methodName
	result.Params = append(result.Params, params...)

	logs := txinfo.Receipt.Logs
	for _, l := range logs {
		logResult, err := self.AnalyzeLog(abi, l)
		if err != nil {
			result.Error += fmt.Sprintf("%s", err)
		}
		result.Logs = append(result.Logs, logResult)
	}
	result.GnosisInit = gnosisResult
}

// this function only compares thee function and param names against
// standard gnosis multisig. No bytecode is compared.
func isGnosisMultisig(method *abi.Method, params []interface{}) bool {
	if method.Name != "submitTransaction" {
		return false
	}
	expectedParams := []string{"destination", "value", "data"}
	for i, input := range method.Inputs {
		if input.Name != expectedParams[i] {
			return false
		}
	}
	return true
}

func (self *TxAnalyzer) gnosisMultisigInitData(a *abi.ABI, params []interface{}) (result *GnosisResult) {
	result = &GnosisResult{
		Contract: "",
		Network:  self.Network,
		Method:   "",
		Params:   []ParamResult{},
		Error:    "",
	}
	var err error
	contract := params[0].(common.Address)
	result.Contract = contract.Hex()
	data := params[2].([]byte)
	if a == nil {
		result.Error = fmt.Sprintf("Cannot get abi of the contract: %s", err)
		return result
	}
	if _, err = a.MethodById(data); err != nil {
		a, _ = ethutils.GetERC20ABI()
	}
	method, err := a.MethodById(data)
	if err != nil {
		result.Error = fmt.Sprintf("Cannot get corresponding method from the ABI: %s", err)
		return result
	}
	// fmt.Printf("    Method: %s\n", method.Name)
	result.Method = method.Name
	ps, err := method.Inputs.UnpackValues(data[4:])
	if err != nil {
		result.Error = fmt.Sprintf("Cannot parse params: %s", err)
		return result
	}
	// fmt.Printf("    Params:\n")
	for i, input := range method.Inputs {
		result.Params = append(result.Params, ParamResult{
			Name:  input.Name,
			Type:  input.Type.String(),
			Value: self.ParamAsStrings(input.Type, ps[i]),
		})
		// fmt.Printf("        %s (%s): ", input.Name, input.Type)
	}
	return result
}

func (self *TxAnalyzer) AnalyzeOffline(txinfo *ethutils.TxInfo, abi *abi.ABI, isContract bool) *TxResult {
	result := NewTxResult()
	result.Network = self.Network
	// fmt.Printf("==========================================Transaction info===============================================================\n")
	// fmt.Printf("tx hash: %s\n", tx)
	result.Hash = txinfo.Tx.Hash().Hex()
	// fmt.Printf("mining status: %s\n", txinfo.Status)
	result.Status = txinfo.Status
	if txinfo.Status == "done" || txinfo.Status == "reverted" {
		self.setBasicTxInfo(*txinfo, result)
		if !isContract {
			// fmt.Printf("tx type: normal\n")
			result.TxType = "normal"
		} else {
			// fmt.Printf("tx type: contract call\n")
			result.TxType = "contract call"
			self.analyzeContractTx(*txinfo, abi, result)
		}
	}
	// fmt.Printf("=========================================================================================================================\n")
	return result
}

func (self *TxAnalyzer) AnalyzeWithABI(tx string, a *abi.ABI) *TxResult {
	txinfo, err := self.reader.TxInfoFromHash(tx)
	if err != nil {
		return &TxResult{
			Error: fmt.Sprintf("getting tx info failed: %s", err),
		}
	}

	code, err := self.reader.GetCode(txinfo.Tx.To().Hex())
	if err != nil {
		return &TxResult{
			Error: fmt.Sprintf("checking tx type failed: %s", err),
		}
	}
	isContract := len(code) > 0

	if isContract {
		return self.AnalyzeOffline(&txinfo, a, true)
	} else {
		return self.AnalyzeOffline(&txinfo, nil, false)
	}
}

// print all info on the tx
func (self *TxAnalyzer) Analyze(tx string) *TxResult {
	txinfo, err := self.reader.TxInfoFromHash(tx)
	if err != nil {
		return &TxResult{
			Error: fmt.Sprintf("getting tx info failed: %s", err),
		}
	}

	code, err := self.reader.GetCode(txinfo.Tx.To().Hex())
	if err != nil {
		return &TxResult{
			Error: fmt.Sprintf("checking tx type failed: %s", err),
		}
	}
	isContract := len(code) > 0

	if isContract {
		abi, err := self.reader.GetABI(txinfo.Tx.To().Hex())
		if err != nil {
			return &TxResult{
				Error: fmt.Sprintf("Cannot get abi of the contract: %s", err),
			}
		}
		return self.AnalyzeOffline(&txinfo, abi, true)
	} else {
		return self.AnalyzeOffline(&txinfo, nil, false)
	}
}

func NewGenericAnalyzer(r *reader.EthReader) *TxAnalyzer {
	return &TxAnalyzer{
		r,
		"mainnet",
	}
}

func NewAnalyzer() *TxAnalyzer {
	return &TxAnalyzer{
		reader.NewEthReader(),
		"mainnet",
	}
}

func NewRopstenAnalyzer() *TxAnalyzer {
	return &TxAnalyzer{
		reader.NewRopstenReader(),
		"ropsten",
	}
}

func NewTomoAnalyzer() *TxAnalyzer {
	return &TxAnalyzer{
		reader.NewTomoReader(),
		"tomo",
	}
}
