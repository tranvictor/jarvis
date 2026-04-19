package safe

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/tranvictor/jarvis/networks"
	"github.com/tranvictor/jarvis/util"
	"github.com/tranvictor/jarvis/util/reader"
)

// SafeContract is a thin reader wrapper around an on-chain Gnosis Safe.
// Mutating operations (execTransaction, approveHash) are not invoked here;
// callers build the calldata themselves and use the standard SignAndBroadcast
// path so the user gets the same gas/dry/no-wait/json-output handling as any
// other transactional command.
type SafeContract struct {
	Address string
	Network networks.Network
	reader  *reader.EthReader
	Abi     *abi.ABI
}

// NewSafeContract constructs a SafeContract bound to the given network.
// It does NOT verify on-chain that the address is actually a Safe; callers
// should check IsGnosisSafe(abi) on the address's ABI first.
func NewSafeContract(address string, network networks.Network) (*SafeContract, error) {
	r, err := util.EthReader(network)
	if err != nil {
		return nil, err
	}
	return &SafeContract{
		Address: address,
		Network: network,
		reader:  r,
		Abi:     GetSafeABI(),
	}, nil
}

// Owners returns the current owner list of the Safe.
func (s *SafeContract) Owners() ([]string, error) {
	owners := new([]common.Address)
	if err := s.reader.ReadContractWithABI(owners, s.Address, s.Abi, "getOwners"); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(*owners))
	for _, o := range *owners {
		out = append(out, o.Hex())
	}
	return out, nil
}

// Threshold returns the current signature threshold of the Safe.
func (s *SafeContract) Threshold() (uint64, error) {
	r := big.NewInt(0)
	if err := s.reader.ReadContractWithABI(&r, s.Address, s.Abi, "getThreshold"); err != nil {
		return 0, err
	}
	return r.Uint64(), nil
}

// Nonce returns the current Safe transaction nonce (the nonce of the next
// SafeTx to be executed, NOT the EOA nonce of the Safe).
func (s *SafeContract) Nonce() (uint64, error) {
	r := big.NewInt(0)
	if err := s.reader.ReadContractWithABI(&r, s.Address, s.Abi, "nonce"); err != nil {
		return 0, err
	}
	return r.Uint64(), nil
}

// Version returns the on-chain VERSION() string, e.g. "1.3.0".
func (s *SafeContract) Version() (string, error) {
	var v string
	if err := s.reader.ReadContractWithABI(&v, s.Address, s.Abi, "VERSION"); err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// DomainSeparator returns the on-chain domainSeparator(). It is the source of
// truth for safeTxHash computation on every Safe version we support.
func (s *SafeContract) DomainSeparator() ([32]byte, error) {
	var raw [32]byte
	if err := s.reader.ReadContractWithABI(&raw, s.Address, s.Abi, "domainSeparator"); err != nil {
		return [32]byte{}, err
	}
	return raw, nil
}

// ApprovedHash returns the approval count for owner over txHash. A non-zero
// result means owner has called approveHash(txHash) on chain.
func (s *SafeContract) ApprovedHash(owner string, txHash [32]byte) (*big.Int, error) {
	r := big.NewInt(0)
	if err := s.reader.ReadContractWithABI(
		&r,
		s.Address, s.Abi, "approvedHashes",
		common.HexToAddress(owner), txHash,
	); err != nil {
		return nil, fmt.Errorf("approvedHashes read: %w", err)
	}
	return r, nil
}
