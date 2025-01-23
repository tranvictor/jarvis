package reader

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	jarviscommon "github.com/tranvictor/jarvis/common"
)

var DO_NOTHING_MC_ONE_RESULT_HANDLER MCOneResultHandler = func(result interface{}) error { return nil }

type MCOneResultHandler func(result interface{}) error

type MultipleCall struct {
	r        *EthReader
	contract string
	mcABI    *abi.ABI
	results  []interface{}
	caddrs   []string
	abis     []*abi.ABI
	methods  []string
	argLists [][]interface{}
	hooks    []MCOneResultHandler
}

func NewMultiCall(r *EthReader, mcContract string) *MultipleCall {
	return &MultipleCall{
		r,
		mcContract,
		jarviscommon.GetMultiCallABI(),
		[]interface{}{},
		[]string{},
		[]*abi.ABI{},
		[]string{},
		[][]interface{}{},
		[]MCOneResultHandler{},
	}
}

func (mc *MultipleCall) RegisterWithHook(
	result interface{},
	hook MCOneResultHandler,
	caddr string,
	abi *abi.ABI,
	method string,
	args ...interface{},
) *MultipleCall {
	mc.results = append(mc.results, result)
	mc.caddrs = append(mc.caddrs, caddr)
	mc.abis = append(mc.abis, abi)
	mc.methods = append(mc.methods, method)
	mc.argLists = append(mc.argLists, args)
	mc.hooks = append(mc.hooks, hook)
	return mc
}

func (mc *MultipleCall) Register(
	result interface{},
	caddr string,
	abi *abi.ABI,
	method string,
	args ...interface{},
) *MultipleCall {
	return mc.RegisterWithHook(
		result,
		DO_NOTHING_MC_ONE_RESULT_HANDLER,
		caddr,
		abi,
		method,
		args...,
	)
}

type multicallres struct {
	BlockNumber *big.Int
	ReturnData  [][]byte
}

type call struct {
	Target   common.Address
	CallData []byte
}

func (mc *MultipleCall) callMCContract(atBlock int64) (block int64, err error) {
	res := multicallres{}

	calls := []call{}
	for i, caddr := range mc.caddrs {
		data, err := mc.abis[i].Pack(mc.methods[i], mc.argLists[i]...)
		if err != nil {
			return 0, err
		}

		calls = append(calls, call{jarviscommon.HexToAddress(caddr), data})
	}

	err = mc.r.ReadHistoryContract(
		atBlock,
		&res,
		mc.contract,
		"aggregate",
		calls,
	)

	if err != nil {
		return 0, fmt.Errorf("reading mc.aggregate failed: %w", err)
	}

	for i := range mc.results {
		err = mc.abis[i].UnpackIntoInterface(
			mc.results[i],
			mc.methods[i],
			res.ReturnData[i],
		)
		if err != nil {
			return 0, fmt.Errorf("unpacking call index %d failed: %w", i, err)
		}
	}
	return res.BlockNumber.Int64(), nil
}

func (mc *MultipleCall) Do(atBlock int64) (block int64, err error) {
	block, err = mc.callMCContract(atBlock)
	if err != nil {
		return 0, fmt.Errorf("calling mc contract failed: %w", err)
	}

	for i, result := range mc.results {
		err = mc.hooks[i](result)
		if err != nil {
			return 0, fmt.Errorf("calling hook at index %d failed: %w", i, err)
		}
	}

	return block, nil
}
