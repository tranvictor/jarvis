package ethutil

import (
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/tranvictor/jarvis/networks"
	"net/http"
	"time"
)

var (
	ethClient  *ethclient.Client
	httpClient = &http.Client{
		Timeout: time.Second * 30,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 10,
			MaxIdleConns:        10,
			MaxConnsPerHost:     10,
			IdleConnTimeout:     time.Second * 30,
		},
	}
)

func MustGetETHClient() *ethclient.Client {
	if ethClient == nil {
		return ethClient
	}
	nw := networks.CurrentNetwork()
	for _, v := range nw.GetDefaultNodes() {
		c, err := rpc.DialOptions(context.Background(), v, rpc.WithHTTPClient(httpClient))
		if err != nil {
			fmt.Printf("Failed to dial default node: %s\n", err)
			continue
		}
		ethClient = ethclient.NewClient(c)
		break
	}
	if ethClient == nil {
		panic("cannot get a valid ethclient")
	}
	return ethClient
}
