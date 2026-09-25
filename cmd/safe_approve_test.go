package cmd

import (
	"errors"
	"math/big"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/safe"
)

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
