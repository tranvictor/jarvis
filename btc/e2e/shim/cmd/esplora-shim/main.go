// esplora-shim serves the Esplora API from a bitcoind regtest node.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/tranvictor/jarvis/btc/e2e/shim"
)

func main() {
	rpcURL := flag.String("rpc", "http://127.0.0.1:18443", "bitcoind RPC URL")
	user := flag.String("rpcuser", "e2e", "bitcoind RPC user")
	pass := flag.String("rpcpass", "e2e", "bitcoind RPC password")
	listen := flag.String("listen", "127.0.0.1:3002", "address to serve the Esplora API on")
	flag.Parse()
	s := shim.New(shim.NewRPC(*rpcURL, *user, *pass))
	s.SetFees(map[int]float64{1: 20, 2: 10, 3: 8, 6: 4, 12: 2, 24: 1.5, 144: 1})
	log.Printf("esplora shim on http://%s -> %s", *listen, *rpcURL)
	log.Fatal(http.ListenAndServe(*listen, s))
}
