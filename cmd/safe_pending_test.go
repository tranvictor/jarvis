package cmd

import (
	"errors"
	"math/big"
	"path/filepath"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/config"
	"github.com/tranvictor/jarvis/safe"
)

const (
	pendingTestSafe = "0x71f8f067348d47cced223eA24D2D77235bea722B"
	pendingTestHash = "0x82c28e25b40c865440a0e89fd9578fe62f629f5fafaa0af0587342f7b4b41efe"
)

type stubCollector struct {
	byHash  map[[32]byte]*safe.PendingTx
	pending []*safe.PendingTx
}

func (s stubCollector) Propose(ethcommon.Address, *safe.SafeTx, [32]byte, ethcommon.Address, []byte) error {
	return errors.New("unused")
}
func (s stubCollector) Confirm([32]byte, ethcommon.Address, []byte) error {
	return errors.New("unused")
}
func (s stubCollector) Get(hash [32]byte) (*safe.PendingTx, error) {
	if p, ok := s.byHash[hash]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}
func (s stubCollector) FindByNonce(ethcommon.Address, uint64) (*safe.PendingTx, error) {
	return nil, errors.New("unused")
}
func (s stubCollector) ListPending(ethcommon.Address) ([]*safe.PendingTx, error) {
	return s.pending, nil
}

func pendingTestHashBytes() [32]byte {
	var h [32]byte
	copy(h[:], ethcommon.FromHex(pendingTestHash))
	return h
}

func TestParseSafeTxHashArg(t *testing.T) {
	h, ok := parseSafeTxHashArg(pendingTestHash)
	if !ok || h != pendingTestHashBytes() {
		t.Fatalf("got ok=%v hash=%x", ok, h)
	}
	if _, ok := parseSafeTxHashArg("0xabc"); ok {
		t.Fatal("short hash should fail")
	}
	if _, ok := parseSafeTxHashArg("7"); ok {
		t.Fatal("nonce should fail")
	}
}

func TestLoadPendingTxNoCollector(t *testing.T) {
	tc := cmdutil.TxContext{Safe: &safe.SafeContract{Address: pendingTestSafe}}
	_, err := loadPendingTx(tc, []string{pendingTestSafe}, "", false)
	if !errors.Is(err, errPendingNoCollector) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadPendingTxOnChainHashFromArg(t *testing.T) {
	tc := cmdutil.TxContext{Safe: &safe.SafeContract{Address: pendingTestSafe}}
	got, err := loadPendingTx(tc, []string{pendingTestSafe, pendingTestHash}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.SafeTxHash != pendingTestHashBytes() {
		t.Fatalf("hash %x", got.SafeTxHash)
	}
	if got.SafeTx != nil {
		t.Fatal("hash-only pending must not invent a SafeTx body")
	}
}

func TestLoadPendingTxOnChainHashFromURL(t *testing.T) {
	h := pendingTestHashBytes()
	tc := cmdutil.TxContext{
		Safe: &safe.SafeContract{Address: pendingTestSafe},
		SafeAppRef: &safe.SafeAppRef{
			SafeAddress: ethcommon.HexToAddress(pendingTestSafe),
			SafeTxHash:  h,
		},
	}
	got, err := loadPendingTx(tc, []string{"https://app.safe.global/"}, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if got.SafeTxHash != h {
		t.Fatalf("hash %x", got.SafeTxHash)
	}
}

func TestLoadPendingTxOnChainNeedsHash(t *testing.T) {
	tc := cmdutil.TxContext{Safe: &safe.SafeContract{Address: pendingTestSafe}}
	_, err := loadPendingTx(tc, []string{pendingTestSafe}, "", true)
	if !errors.Is(err, errPendingNeedHash) {
		t.Fatalf("got %v", err)
	}
}

func TestLoadPendingTxFromService(t *testing.T) {
	h := pendingTestHashBytes()
	want := &safe.PendingTx{SafeTxHash: h}
	tc := cmdutil.TxContext{
		Safe:      &safe.SafeContract{Address: pendingTestSafe},
		Collector: stubCollector{byHash: map[[32]byte]*safe.PendingTx{h: want}},
	}
	got, err := loadPendingTx(tc, []string{pendingTestSafe, pendingTestHash}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadPendingTxFromFile(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tx.json")
	tx := safe.NewSafeTx(ethcommon.HexToAddress("0x1111111111111111111111111111111111111111"), big.NewInt(0), nil, safe.OpCall, 1)
	h := pendingTestHashBytes()
	if err := safe.WriteTxFile(path, pendingTestSafe, 1, tx, h, nil); err != nil {
		t.Fatal(err)
	}
	tc := cmdutil.TxContext{Safe: &safe.SafeContract{Address: pendingTestSafe}}
	got, err := loadPendingTx(tc, nil, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.SafeTxHash != h {
		t.Fatalf("hash %x", got.SafeTxHash)
	}
	if got.SafeTx == nil || got.SafeTx.Nonce.Uint64() != 1 {
		t.Fatalf("body %+v", got.SafeTx)
	}
}

func TestLoadPendingTxFileWrongSafe(t *testing.T) {
	if err := config.SetNetwork("mainnet"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "tx.json")
	tx := safe.NewSafeTx(ethcommon.HexToAddress("0x1111111111111111111111111111111111111111"), big.NewInt(0), nil, safe.OpCall, 1)
	if err := safe.WriteTxFile(path, pendingTestSafe, 1, tx, pendingTestHashBytes(), nil); err != nil {
		t.Fatal(err)
	}
	tc := cmdutil.TxContext{Safe: &safe.SafeContract{Address: "0x2222222222222222222222222222222222222222"}}
	_, err := loadPendingTx(tc, nil, path, false)
	if err == nil {
		t.Fatal("expected safe mismatch")
	}
}
