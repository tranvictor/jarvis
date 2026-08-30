package safe

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// SubmitProposal persists a newly signed SafeTx either to a local file
// or to a SignatureCollector. File mode does not also POST to the
// collector (the file is the source of truth).
func SubmitProposal(
	c SignatureCollector,
	file string,
	safeAddr common.Address,
	chainID uint64,
	stx *SafeTx,
	hash [32]byte,
	owner common.Address,
	sig []byte,
) error {
	if file != "" {
		return WriteTxFile(file, safeAddr.Hex(), chainID, stx, hash, []OwnerSig{{Owner: owner, Sig: sig}})
	}
	if c == nil {
		return fmt.Errorf("no signature collector")
	}
	return c.Propose(safeAddr, stx, hash, owner, sig)
}
