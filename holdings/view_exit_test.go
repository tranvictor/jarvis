package holdings

import (
	"strings"
	"testing"
)

func TestDegenPrintsLiquidityPayload(t *testing.T) {
	usdt := 5000.0
	knc := 1200.0
	eth := 2000.0
	book := &Book{
		Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045",
		MinUSD:  1,
		Positions: []*Position{
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xdac17f958d2ee523a2206206994597c13d831ec7", Symbol: "USDT", Amount: "5000", Value: &usdt, Mark: "sell", ExitTier: "none", ExitQuoted: true, ExitMethod: "GET", ExitURL: "https://api.kyberswap.com/data/v1/liquidity/1/pair?tokenIn=0xdac17f958d2ee523a2206206994597c13d831ec7&tokenOut=0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", ExitRaw: `{"code":0,"data":{"tier":"none","score":0,"venueCount":0},"message":"OK"}`},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xdefa4e8a7bcba345f687a2f1456f5edd9ce97202", Symbol: "KNC", Amount: "100", Value: &knc, Mark: "sell", ExitTier: "medium", ExitQuoted: true, ExitMethod: "GET", ExitURL: "https://api.kyberswap.com/data/v1/liquidity/1/pair?tokenIn=0xdefa4e8a7bcba345f687a2f1456f5edd9ce97202&tokenOut=0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", ExitRaw: `{"code":0,"data":{"score":115500.69,"tier":"medium","venueCount":4,"volume24hUsd":14.005},"message":"OK"}`},
			{ChainID: 1, ChainName: "Ethereum", TokenAddress: "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Symbol: "ETH", Amount: "1", Value: &eth, Native: true, Mark: "sell"},
		},
	}

	off := renderPlain(t, book, false)
	if strings.Contains(off, "volume24hUsd") || strings.Contains(off, "not quoted") || strings.Contains(off, "liquidity/1/pair") {
		t.Fatalf("liquidity trace printed without -x:\n%s", off)
	}
	if !strings.Contains(off, "none") || !strings.Contains(off, "medium") {
		t.Fatalf("exit cells missing:\n%s", off)
	}

	on := renderPlain(t, book, true)
	for _, want := range []string{
		`Ethereum USDT  GET https://api.kyberswap.com/data/v1/liquidity/1/pair?tokenIn=0xdac17f958d2ee523a2206206994597c13d831ec7&tokenOut=0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee`,
		`{"code":0,"data":{"tier":"none","score":0,"venueCount":0},"message":"OK"}`,
		`Ethereum KNC  GET https://api.kyberswap.com/data/v1/liquidity/1/pair?tokenIn=0xdefa4e8a7bcba345f687a2f1456f5edd9ce97202&tokenOut=0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee`,
		`{"code":0,"data":{"score":115500.69,"tier":"medium","venueCount":4,"volume24hUsd":14.005},"message":"OK"}`,
		`Ethereum ETH  not quoted`,
	} {
		if !strings.Contains(on, want) {
			t.Fatalf("missing %q\n%s", want, on)
		}
	}
}
