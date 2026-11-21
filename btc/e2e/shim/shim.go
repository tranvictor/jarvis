package shim

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Mode int

const (
	Normal Mode = iota
	// ServerError makes every request fail with HTTP 500.
	ServerError
	// BadJSON makes every JSON endpoint answer with a body that is not JSON.
	BadJSON
	// StaleUTXOs serves the coin lists captured by Freeze, as if the
	// indexer lagged behind the node.
	StaleUTXOs
)

type txRec struct {
	txid   string
	height int64
	outs   []outRec
	ins    []inRec
}

type outRec struct {
	addr  string
	value int64
	n     uint32
}

type inRec struct {
	prevTxid string
	prevN    uint32
	addr     string
	value    int64
	coinbase bool
}

type Shim struct {
	rpc *RPC

	mu          sync.Mutex
	mode        Mode
	fees        map[string]float64
	cache       map[int64][]txRec
	indexed     int64
	frozen      map[string]string
	frozenStats map[string]string
	log         []string
}

func New(rpc *RPC) *Shim {
	return &Shim{rpc: rpc, cache: map[int64][]txRec{}, indexed: -1, frozen: map[string]string{}, frozenStats: map[string]string{}}
}

func (s *Shim) SetMode(m Mode) {
	s.mu.Lock()
	s.mode = m
	s.mu.Unlock()
}

// SetFees sets what /fee-estimates answers: confirmation target -> sat/vB.
// A nil map answers {} like a regtest node with no fee history.
func (s *Shim) SetFees(f map[int]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fees = map[string]float64{}
	for k, v := range f {
		s.fees[strconv.Itoa(k)] = v
	}
}

// Freeze captures the current UTXO list of addr; StaleUTXOs mode serves it.
func (s *Shim) Freeze(addr string) error {
	body, status := s.utxoBody(addr)
	if status != 200 {
		return fmt.Errorf("freeze %s: %s", addr, body)
	}
	stats, status := s.addressBody(addr)
	if status != 200 {
		return fmt.Errorf("freeze %s: %s", addr, stats)
	}
	s.mu.Lock()
	s.frozen[addr] = body
	s.frozenStats[addr] = stats
	s.mu.Unlock()
	return nil
}

// Requests returns the "METHOD /path" lines served so far.
func (s *Shim) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.log...)
}

func (s *Shim) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	mode := s.mode
	s.log = append(s.log, r.Method+" "+r.URL.Path)
	s.mu.Unlock()

	if mode == ServerError {
		http.Error(w, "indexer exploded", http.StatusInternalServerError)
		return
	}
	if mode == BadJSON && r.Method == http.MethodGet && !strings.HasSuffix(r.URL.Path, "/hex") && !strings.HasSuffix(r.URL.Path, "/height") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "<html>502 bad gateway</html>")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	switch {
	case r.Method == http.MethodGet && path == "fee-estimates":
		s.mu.Lock()
		f := s.fees
		s.mu.Unlock()
		if f == nil {
			f = map[string]float64{}
		}
		writeJSON(w, f)
	case r.Method == http.MethodGet && path == "blocks/tip/height":
		var h int64
		if err := s.rpc.CallInto(&h, "getblockcount"); err != nil {
			fail(w, err)
			return
		}
		_, _ = io.WriteString(w, strconv.FormatInt(h, 10))
	case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "address":
		if mode == StaleUTXOs {
			s.mu.Lock()
			body, ok := s.frozenStats[parts[1]]
			s.mu.Unlock()
			if ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
				return
			}
		}
		s.address(w, parts[1])
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "address" && parts[2] == "utxo":
		if mode == StaleUTXOs {
			s.mu.Lock()
			body, ok := s.frozen[parts[1]]
			s.mu.Unlock()
			if ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
				return
			}
		}
		body, status := s.utxoBody(parts[1])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "tx" && parts[2] == "hex":
		var hexTx string
		if err := s.rpc.CallInto(&hexTx, "getrawtransaction", parts[1]); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, hexTx)
	case r.Method == http.MethodGet && len(parts) == 3 && parts[0] == "tx" && parts[2] == "status":
		var tx struct {
			Confirmations int64  `json:"confirmations"`
			BlockHash     string `json:"blockhash"`
		}
		if err := s.rpc.CallInto(&tx, "getrawtransaction", parts[1], true); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"confirmed": tx.Confirmations > 0})
	case r.Method == http.MethodPost && path == "tx":
		raw, _ := io.ReadAll(r.Body)
		var txid string
		if err := s.rpc.CallInto(&txid, "sendrawtransaction", strings.TrimSpace(string(raw))); err != nil {
			http.Error(w, "sendrawtransaction RPC error: "+err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, txid)
	default:
		http.NotFound(w, r)
	}
}

func fail(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func sats(v float64) int64 { return int64(math.Round(v * 1e8)) }

type vout struct {
	Value        float64 `json:"value"`
	N            uint32  `json:"n"`
	ScriptPubKey struct {
		Address string `json:"address"`
	} `json:"scriptPubKey"`
}

type vin struct {
	Txid     string `json:"txid"`
	Vout     uint32 `json:"vout"`
	Coinbase string `json:"coinbase"`
	Prevout  *struct {
		Value        float64 `json:"value"`
		ScriptPubKey struct {
			Address string `json:"address"`
		} `json:"scriptPubKey"`
	} `json:"prevout"`
}

type rawTx struct {
	Txid string `json:"txid"`
	Vin  []vin  `json:"vin"`
	Vout []vout `json:"vout"`
}

func toRec(t rawTx, height int64) txRec {
	rec := txRec{txid: t.Txid, height: height}
	for _, o := range t.Vout {
		rec.outs = append(rec.outs, outRec{addr: o.ScriptPubKey.Address, value: sats(o.Value), n: o.N})
	}
	for _, i := range t.Vin {
		in := inRec{prevTxid: i.Txid, prevN: i.Vout, coinbase: i.Coinbase != ""}
		if i.Prevout != nil {
			in.addr = i.Prevout.ScriptPubKey.Address
			in.value = sats(i.Prevout.Value)
		}
		rec.ins = append(rec.ins, in)
	}
	return rec
}

// snapshot returns every confirmed and unconfirmed transaction record.
func (s *Shim) snapshot() (chain, mempool []txRec, err error) {
	var tip int64
	if err = s.rpc.CallInto(&tip, "getblockcount"); err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for h := s.indexed + 1; h <= tip; h++ {
		var hash string
		if err = s.rpc.CallInto(&hash, "getblockhash", h); err != nil {
			return
		}
		var blk struct {
			Tx []rawTx `json:"tx"`
		}
		if err = s.rpc.CallInto(&blk, "getblock", hash, 3); err != nil {
			return
		}
		recs := make([]txRec, 0, len(blk.Tx))
		for _, t := range blk.Tx {
			recs = append(recs, toRec(t, h))
		}
		s.cache[h] = recs
		s.indexed = h
	}
	for h := int64(0); h <= s.indexed; h++ {
		chain = append(chain, s.cache[h]...)
	}
	var ids []string
	if err = s.rpc.CallInto(&ids, "getrawmempool"); err != nil {
		return
	}
	for _, id := range ids {
		var t rawTx
		if err = s.rpc.CallInto(&t, "getrawtransaction", id, 2); err != nil {
			return
		}
		mempool = append(mempool, toRec(t, -1))
	}
	// getrawtransaction does not report prevouts of unconfirmed spends, so
	// resolve them from the parent transactions.
	outs := map[string]map[uint32]outRec{}
	for _, set := range [][]txRec{chain, mempool} {
		for _, r := range set {
			m := map[uint32]outRec{}
			for _, o := range r.outs {
				m[o.n] = o
			}
			outs[r.txid] = m
		}
	}
	for i := range mempool {
		for j := range mempool[i].ins {
			in := &mempool[i].ins[j]
			if in.coinbase || in.addr != "" {
				continue
			}
			parent, ok := outs[in.prevTxid]
			if !ok {
				err = fmt.Errorf("parent %s of mempool tx %s is unknown", in.prevTxid, mempool[i].txid)
				return
			}
			in.addr, in.value = parent[in.prevN].addr, parent[in.prevN].value
		}
	}
	return
}

func sumFor(recs []txRec, addr string) (funded, spent int64, txids map[string]bool) {
	txids = map[string]bool{}
	for _, r := range recs {
		for _, o := range r.outs {
			if o.addr == addr {
				funded += o.value
				txids[r.txid] = true
			}
		}
		for _, i := range r.ins {
			if i.addr == addr && !i.coinbase {
				spent += i.value
				txids[r.txid] = true
			}
		}
	}
	return
}

func (s *Shim) address(w http.ResponseWriter, addr string) {
	body, status := s.addressBody(addr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (s *Shim) addressBody(addr string) (string, int) {
	chain, mempool, err := s.snapshot()
	if err != nil {
		return err.Error(), http.StatusInternalServerError
	}
	cf, cs, ct := sumFor(chain, addr)
	mf, ms, mt := sumFor(mempool, addr)
	b, _ := json.Marshal(map[string]any{
		"address": addr,
		"chain_stats": map[string]any{
			"funded_txo_sum": cf, "spent_txo_sum": cs, "tx_count": len(ct),
		},
		"mempool_stats": map[string]any{
			"funded_txo_sum": mf, "spent_txo_sum": ms, "tx_count": len(mt),
		},
	})
	return string(b), http.StatusOK
}

func (s *Shim) utxoBody(addr string) (string, int) {
	chain, mempool, err := s.snapshot()
	if err != nil {
		return err.Error(), http.StatusInternalServerError
	}
	spent := map[string]bool{}
	for _, set := range [][]txRec{chain, mempool} {
		for _, r := range set {
			for _, i := range r.ins {
				if !i.coinbase {
					spent[fmt.Sprintf("%s:%d", i.prevTxid, i.prevN)] = true
				}
			}
		}
	}
	type utxo struct {
		Txid   string `json:"txid"`
		Vout   uint32 `json:"vout"`
		Value  int64  `json:"value"`
		Status struct {
			Confirmed   bool  `json:"confirmed"`
			BlockHeight int64 `json:"block_height,omitempty"`
		} `json:"status"`
	}
	out := []utxo{}
	for _, set := range [][]txRec{chain, mempool} {
		for _, r := range set {
			for _, o := range r.outs {
				if o.addr != addr || spent[fmt.Sprintf("%s:%d", r.txid, o.n)] {
					continue
				}
				u := utxo{Txid: r.txid, Vout: o.n, Value: o.value}
				u.Status.Confirmed = r.height >= 0
				if r.height >= 0 {
					u.Status.BlockHeight = r.height
				}
				out = append(out, u)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Txid != out[j].Txid {
			return out[i].Txid < out[j].Txid
		}
		return out[i].Vout < out[j].Vout
	})
	b, _ := json.Marshal(out)
	return string(b), http.StatusOK
}
