package txanalyzer

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/tranvictor/jarvis/networks"
)

func TestAnalyzeLogEmptyTopics(t *testing.T) {
	network, err := networks.GetNetwork("mainnet")
	if err != nil {
		t.Fatal(err)
	}
	ta := NewGenericAnalyzer(nil, network)
	_, err = ta.AnalyzeLog(nil, &types.Log{
		Address: common.HexToAddress("0x48B8419B2Bc0fB63Ee96e3a370e30B200cC2e672"),
		Topics:  nil,
	})
	if err == nil {
		t.Fatal("expected error for log with no topics")
	}
}
