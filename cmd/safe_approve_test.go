package cmd

import (
	"errors"
	"math/big"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	cmdutil "github.com/tranvictor/jarvis/cmd/util"
	"github.com/tranvictor/jarvis/safe"
)

func TestVerifyPendingSafeTxHashSkipsNilBody(t *testing.T) {
	pending := &safe.PendingTx{SafeTxHash: pendingTestHashBytes()}
	if _, err := verifyPendingSafeTxHash(pending, [32]byte{1}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyPendingSafeTxHashMatchAndMismatch(t *testing.T) {
	stx := safe.NewSafeTx(ethcommon.HexToAddress("0x1111111111111111111111111111111111111111"), big.NewInt(0), nil, safe.OpCall, 3)
	var domain [32]byte
	domain[0] = 0xab
	hash := stx.SafeTxHash(domain)
	pending := &safe.PendingTx{SafeTx: stx, SafeTxHash: hash}

	if _, err := verifyPendingSafeTxHash(pending, domain); err != nil {
		t.Fatal(err)
	}

	pending.SafeTxHash = pendingTestHashBytes()
	expected, err := verifyPendingSafeTxHash(pending, domain)
	if !errors.Is(err, errPendingHashMismatch) {
		t.Fatalf("got %v", err)
	}
	if expected != hash {
		t.Fatalf("expected %x want %x", expected, hash)
	}
}

func TestOwnerAlreadySigned(t *testing.T) {
	me := ethcommon.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	other := ethcommon.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	pending := &safe.PendingTx{
		Sigs: []safe.OwnerSig{
			{Owner: other, Sig: []byte{1}},
		},
	}
	if _, found := ownerAlreadySigned(pending, me); found {
		t.Fatal("should not find me")
	}

	pending.Sigs = append(pending.Sigs, safe.OwnerSig{Owner: me, Sig: append(make([]byte, 64), 27)})
	onChain, found := ownerAlreadySigned(pending, me)
	if !found || onChain {
		t.Fatalf("off-chain: onChain=%v found=%v", onChain, found)
	}

	pending.Sigs[1] = safe.OnChainApprovalSig(me)
	onChain, found = ownerAlreadySigned(pending, me)
	if !found || !onChain {
		t.Fatalf("on-chain: onChain=%v found=%v", onChain, found)
	}
}

func TestPendingWithNewSig(t *testing.T) {
	me := ethcommon.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	pending := &safe.PendingTx{
		Sigs: []safe.OwnerSig{{Owner: ethcommon.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), Sig: []byte{1}}},
	}
	got := pendingWithNewSig(pending, me, []byte{2})
	if len(pending.Sigs) != 1 {
		t.Fatal("must not mutate original")
	}
	if len(got.Sigs) != 2 || got.Sigs[1].Owner != me {
		t.Fatalf("got %+v", got.Sigs)
	}
}

type confirmCollector struct {
	stubCollector
	gotHash [32]byte
	gotMe   ethcommon.Address
	gotSig  []byte
}

func (c *confirmCollector) Confirm(hash [32]byte, owner ethcommon.Address, sig []byte) error {
	c.gotHash = hash
	c.gotMe = owner
	c.gotSig = sig
	return nil
}

func TestPersistApprovalUsesCollector(t *testing.T) {
	h := pendingTestHashBytes()
	me := ethcommon.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	col := &confirmCollector{}
	tc := cmdutil.TxContext{Collector: col}
	pending := &safe.PendingTx{SafeTxHash: h}
	if err := persistApproval(tc, pendingTestSafe, pending, me, []byte{9}, "", 1); err != nil {
		t.Fatal(err)
	}
	if col.gotHash != h || col.gotMe != me || len(col.gotSig) != 1 || col.gotSig[0] != 9 {
		t.Fatalf("confirm %+v %+v %v", col.gotHash, col.gotMe, col.gotSig)
	}
}
