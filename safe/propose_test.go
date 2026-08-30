package safe

import (
	"errors"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

type proposeCollector struct {
	called bool
	addr   common.Address
	hash   [32]byte
	owner  common.Address
	sig    []byte
}

func (p *proposeCollector) Propose(safe common.Address, _ *SafeTx, hash [32]byte, owner common.Address, sig []byte) error {
	p.called = true
	p.addr = safe
	p.hash = hash
	p.owner = owner
	p.sig = sig
	return nil
}
func (p *proposeCollector) Confirm([32]byte, common.Address, []byte) error {
	return errors.New("unused")
}
func (p *proposeCollector) Get([32]byte) (*PendingTx, error) { return nil, errors.New("unused") }
func (p *proposeCollector) FindByNonce(common.Address, uint64) (*PendingTx, error) {
	return nil, errors.New("unused")
}
func (p *proposeCollector) ListPending(common.Address) ([]*PendingTx, error) {
	return nil, errors.New("unused")
}

func TestSubmitProposalToCollector(t *testing.T) {
	stx := NewSafeTx(common.HexToAddress("0x1111111111111111111111111111111111111111"), big.NewInt(1), nil, OpCall, 0)
	var hash [32]byte
	hash[0] = 0xab
	owner := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	safeAddr := common.HexToAddress("0x71f8f067348d47cced223eA24D2D77235bea722B")
	col := &proposeCollector{}
	if err := SubmitProposal(col, "", safeAddr, 1, stx, hash, owner, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}
	if !col.called || col.addr != safeAddr || col.hash != hash || col.owner != owner {
		t.Fatalf("propose %+v", col)
	}
}

func TestSubmitProposalFileSkipsCollector(t *testing.T) {
	stx := NewSafeTx(common.HexToAddress("0x1111111111111111111111111111111111111111"), big.NewInt(0), nil, OpCall, 4)
	var hash [32]byte
	hash[1] = 0xcd
	owner := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	safeAddr := common.HexToAddress("0x71f8f067348d47cced223eA24D2D77235bea722B")
	col := &proposeCollector{}
	path := filepath.Join(t.TempDir(), "tx.json")
	if err := SubmitProposal(col, path, safeAddr, 1, stx, hash, owner, []byte{9}); err != nil {
		t.Fatal(err)
	}
	if col.called {
		t.Fatal("file mode must not POST to the collector")
	}
	tf, err := ReadTxFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if tf.ChainID != 1 {
		t.Fatalf("chain %d", tf.ChainID)
	}
}

func TestSubmitProposalNilCollector(t *testing.T) {
	stx := NewSafeTx(common.Address{}, big.NewInt(0), nil, OpCall, 0)
	err := SubmitProposal(nil, "", common.Address{}, 1, stx, [32]byte{}, common.Address{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
