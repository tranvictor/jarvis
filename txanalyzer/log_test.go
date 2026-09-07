package txanalyzer

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/tranvictor/jarvis/networks"
)

func TestAnalyzeLogEmptyTopics(t *testing.T) {
	network, err := networks.GetNetwork("mainnet")
	if err != nil {
		t.Fatal(err)
	}
	ta := NewGenericAnalyzer(nil, network)
	_, err = ta.AnalyzeLog(nil, nil, &types.Log{
		Address: common.HexToAddress("0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"),
		Topics:  nil,
	})
	if err == nil {
		t.Fatal("expected error for log with no topics")
	}
}

// failingLookup stands in for a block explorer that has no ABI for anything.
func failingLookup(addr string, _ networks.Network) (*abi.ABI, error) {
	return nil, fmt.Errorf("no abi for %s", addr)
}

func TestAnalyzeLogFallsBackToWellKnownEvents(t *testing.T) {
	network, _ := networks.GetNetwork("mainnet")
	ta := NewGenericAnalyzer(nil, network)
	transferSig := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	from := common.HexToAddress("0x9642b23Ed1E01Df1092B92641051881a322F5D4E")
	to := common.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
	value := common.LeftPadBytes(big.NewInt(1000).Bytes(), 32)

	erc20, err := ta.AnalyzeLog(failingLookup, nil, &types.Log{
		Address: common.HexToAddress("0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"),
		Topics:  []common.Hash{transferSig, common.BytesToHash(from.Bytes()), common.BytesToHash(to.Bytes())},
		Data:    value,
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if erc20.Name != "Transfer" || len(erc20.Topics) != 2 || len(erc20.Data) != 1 {
		t.Fatalf("erc20 transfer not decoded from well-known ABI: %+v", erc20)
	}
	if erc20.Topics[0].Name != "from" || erc20.Data[0].Values[0].Raw != "1000" {
		t.Fatalf("unexpected decoded fields: %+v", erc20)
	}

	erc721, err := ta.AnalyzeLog(failingLookup, nil, &types.Log{
		Address: common.HexToAddress("0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"),
		Topics:  []common.Hash{transferSig, common.BytesToHash(from.Bytes()), common.BytesToHash(to.Bytes()), common.BigToHash(big.NewInt(7))},
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if erc721.Name != "Transfer" || len(erc721.Topics) != 3 || erc721.Topics[2].Name != "tokenId" || erc721.Topics[2].Value.Raw != "7" {
		t.Fatalf("erc721 transfer not decoded: %+v", erc721)
	}
}

func TestAnalyzeLogKeepsUndecodedLogsRaw(t *testing.T) {
	network, _ := networks.GetNetwork("mainnet")
	ta := NewGenericAnalyzer(nil, network)
	topic0 := crypto.Keccak256Hash([]byte("Something(uint256)"))
	res, err := ta.AnalyzeLog(failingLookup, nil, &types.Log{
		Address: common.HexToAddress("0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"),
		Topics:  []common.Hash{topic0, common.BigToHash(big.NewInt(3))},
		Data:    common.LeftPadBytes([]byte{1}, 32),
	})
	if err != nil {
		t.Fatalf("undecodable logs must not be errors: %s", err)
	}
	if res.Name != "" {
		t.Fatalf("name should stay empty, got %q", res.Name)
	}
	if len(res.Topics) != 2 || res.Topics[0].Name != "topic0" || res.Topics[0].Value.Raw != topic0.Hex() {
		t.Fatalf("raw topics missing: %+v", res.Topics)
	}
	if len(res.Data) != 1 || res.Data[0].Name != "data" {
		t.Fatalf("raw data missing: %+v", res.Data)
	}
}

func TestDecodeRevertData(t *testing.T) {
	stringT, _ := abi.NewType("string", "", nil)
	msg, _ := (abi.Arguments{{Type: stringT}}).Pack("Insufficient output")
	if got := DecodeRevertData(append(common.FromHex("0x08c379a0"), msg...), nil); got != `"Insufficient output"` {
		t.Fatalf("Error(string): %q", got)
	}
	uintT, _ := abi.NewType("uint256", "", nil)
	code, _ := (abi.Arguments{{Type: uintT}}).Pack(big.NewInt(0x11))
	if got := DecodeRevertData(append(common.FromHex("0x4e487b71"), code...), nil); got != "panic 0x11: arithmetic overflow or underflow" {
		t.Fatalf("Panic: %q", got)
	}
	custom := abiFromJSON(t, `[{"type":"error","name":"SlippageExceeded","inputs":[{"name":"minOut","type":"uint256"},{"name":"got","type":"uint256"}]}]`)
	e := custom.Errors["SlippageExceeded"]
	args, _ := e.Inputs.Pack(big.NewInt(100), big.NewInt(90))
	if got := DecodeRevertData(append(e.ID.Bytes()[:4], args...), custom); got != "SlippageExceeded(100, 90)" {
		t.Fatalf("custom error: %q", got)
	}
	if got := DecodeRevertData(common.FromHex("0x97a6f3b9"), nil); got != "custom error 0x97a6f3b9" {
		t.Fatalf("unknown selector: %q", got)
	}
	if got := DecodeRevertData(nil, nil); got != "" {
		t.Fatalf("empty payload: %q", got)
	}
}
