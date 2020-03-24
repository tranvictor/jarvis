package txanalyzer

type ParamResult struct {
	Name  string
	Type  string
	Value []string
}

type TopicResult struct {
	Name  string
	Value []string
}

type LogResult struct {
	Name   string
	Topics []TopicResult
	Data   []ParamResult
}

type GnosisResult struct {
	Contract string
	Network  string
	Method   string
	Params   []ParamResult
	Error    string
}

type TxResult struct {
	Hash     string
	Network  string
	Status   string
	From     string
	Value    string
	To       string
	Nonce    string
	GasPrice string
	GasLimit string
	TxType   string

	Contract string
	Method   string
	Params   []ParamResult
	Logs     []LogResult

	GnosisInit *GnosisResult

	Completed bool
	Error     string
}

func NewTxResult() *TxResult {
	return &TxResult{
		Hash:       "",
		Network:    "mainnet",
		Status:     "",
		From:       "",
		Value:      "",
		To:         "",
		Nonce:      "",
		GasPrice:   "",
		GasLimit:   "",
		TxType:     "",
		Contract:   "",
		Method:     "",
		Params:     []ParamResult{},
		Logs:       []LogResult{},
		GnosisInit: nil,
		Completed:  false,
		Error:      "",
	}
}
