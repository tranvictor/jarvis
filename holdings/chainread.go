package holdings

import (
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

const (
	amountChain = "chain"
	amountKyber = "kyber"

	// Multicall3 is deployed at this address on every chain that uses
	// the canonical CREATE2 deployment.
	multicall3Addr = "0xcA11bde05977b3631167028862bE2a173976CA11"
)

const multicallABIJSON = `[{"inputs":[{"components":[{"internalType":"address","name":"target","type":"address"},{"internalType":"bool","name":"allowFailure","type":"bool"},{"internalType":"bytes","name":"callData","type":"bytes"}],"internalType":"struct Multicall3.Call3[]","name":"calls","type":"tuple[]"}],"name":"aggregate3","outputs":[{"components":[{"internalType":"bool","name":"success","type":"bool"},{"internalType":"bytes","name":"returnData","type":"bytes"}],"internalType":"struct Multicall3.Result[]","name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"},{"inputs":[{"internalType":"address","name":"addr","type":"address"}],"name":"getEthBalance","outputs":[{"internalType":"uint256","name":"balance","type":"uint256"}],"stateMutability":"view","type":"function"}]`

const balanceABIJSON = `[{"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"stateMutability":"view","type":"function"}]`

type mcCall struct {
	Target       common.Address `abi:"target"`
	AllowFailure bool           `abi:"allowFailure"`
	CallData     []byte         `abi:"callData"`
}

type mcResult struct {
	Success    bool   `abi:"success"`
	ReturnData []byte `abi:"returnData"`
}

var (
	abiOnce sync.Once
	mcABI   abi.ABI
	balABI  abi.ABI
	abiErr  error
)

func loadABIs() error {
	abiOnce.Do(func() {
		var err error
		mcABI, err = abi.JSON(strings.NewReader(multicallABIJSON))
		if err != nil {
			abiErr = err
			return
		}
		balABI, err = abi.JSON(strings.NewReader(balanceABIJSON))
		abiErr = err
	})
	return abiErr
}

func (l *Loader) reader(chainID int) (*reader.EthReader, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r, ok := l.readers[chainID]; ok {
		return r, nil
	}
	if l.noReader[chainID] {
		return nil, errNoReader
	}
	n, err := networks.GetNetworkByID(uint64(chainID))
	if err != nil {
		l.noReader[chainID] = true
		return nil, err
	}
	r, err := util.EthReader(n)
	if err != nil {
		l.noReader[chainID] = true
		return nil, err
	}
	l.readers[chainID] = r
	return r, nil
}

func (l *Loader) readAmounts(wallet string, positions []*Position) {
	by := map[int][]*Position{}
	for _, p := range positions {
		by[p.ChainID] = append(by[p.ChainID], p)
	}
	var wg sync.WaitGroup
	for id, ps := range by {
		wg.Add(1)
		go func(id int, ps []*Position) {
			defer wg.Done()
			r, err := l.reader(id)
			if err != nil {
				return
			}
			if err := multicallBalances(r, wallet, ps); err != nil {
				individualBalances(r, wallet, ps)
			}
		}(id, ps)
	}
	wg.Wait()
}

func multicallBalances(r *reader.EthReader, wallet string, ps []*Position) error {
	if err := loadABIs(); err != nil {
		return err
	}
	user := common.HexToAddress(wallet)
	type job struct {
		positions []*Position
		call      mcCall
		native    bool
	}
	var jobs []job
	var natives []*Position
	for _, p := range ps {
		if p.Native {
			natives = append(natives, p)
			continue
		}
		data, err := balABI.Pack("balanceOf", user)
		if err != nil {
			continue
		}
		jobs = append(jobs, job{
			positions: []*Position{p},
			call: mcCall{
				Target:       common.HexToAddress(p.TokenAddress),
				AllowFailure: true,
				CallData:     data,
			},
		})
	}
	if len(natives) > 0 {
		data, err := mcABI.Pack("getEthBalance", user)
		if err == nil {
			jobs = append(jobs, job{
				positions: natives,
				native:    true,
				call: mcCall{
					Target:       common.HexToAddress(multicall3Addr),
					AllowFailure: true,
					CallData:     data,
				},
			})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	const chunk = 80
	for start := 0; start < len(jobs); start += chunk {
		end := start + chunk
		if end > len(jobs) {
			end = len(jobs)
		}
		part := jobs[start:end]
		calls := make([]mcCall, len(part))
		for i := range part {
			calls[i] = part[i].call
		}
		var results []mcResult
		err := r.ReadContractWithABI(&results, multicall3Addr, &mcABI, "aggregate3", calls)
		if err != nil {
			return err
		}
		if len(results) != len(part) {
			return errShortMulticall
		}
		for i, res := range results {
			if !res.Success || len(res.ReturnData) == 0 {
				continue
			}
			bal, ok := unpackUint(res.ReturnData, part[i].native)
			if !ok {
				continue
			}
			for _, p := range part[i].positions {
				applyRaw(p, bal)
			}
		}
	}
	return nil
}

func unpackUint(data []byte, native bool) (*big.Int, bool) {
	var parsed abi.ABI
	method := "balanceOf"
	if native {
		parsed = mcABI
		method = "getEthBalance"
	} else {
		parsed = balABI
	}
	vals, err := parsed.Unpack(method, data)
	if err != nil || len(vals) == 0 {
		return nil, false
	}
	switch v := vals[0].(type) {
	case *big.Int:
		return v, true
	default:
		return nil, false
	}
}

func individualBalances(r *reader.EthReader, wallet string, ps []*Position) {
	var natives []*Position
	var tokens []*Position
	for _, p := range ps {
		if p.AmountSource == amountChain {
			continue
		}
		if p.Native {
			natives = append(natives, p)
			continue
		}
		tokens = append(tokens, p)
	}
	if len(natives) > 0 {
		if bal, err := r.GetBalance(wallet); err == nil {
			for _, p := range natives {
				applyRaw(p, bal)
			}
		}
	}
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for _, p := range tokens {
		wg.Add(1)
		sem <- struct{}{}
		go func(p *Position) {
			defer wg.Done()
			defer func() { <-sem }()
			bal, err := r.ERC20Balance(p.TokenAddress, wallet)
			if err != nil {
				return
			}
			applyRaw(p, bal)
		}(p)
	}
	wg.Wait()
}

func applyRaw(p *Position, raw *big.Int) {
	if raw == nil {
		return
	}
	p.RawAmount = raw.String()
	p.Amount = humanAmount(raw, p.Decimals)
	p.AmountSource = amountChain
}
