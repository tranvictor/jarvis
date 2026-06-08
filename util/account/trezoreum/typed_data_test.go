package trezoreum

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
)

func TestTrezorFieldTypePermit(t *testing.T) {
	types := map[string][]apitypes.Type{
		"Permit": {
			{Name: "owner", Type: "address"},
			{Name: "spender", Type: "address"},
			{Name: "value", Type: "uint256"},
			{Name: "nonce", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		},
	}
	ft, err := trezorFieldType("uint256", types)
	if err != nil {
		t.Fatal(err)
	}
	if ft.GetDataType() != trezor.EthereumTypedDataStructAck_UINT {
		t.Fatalf("expected UINT, got %v", ft.GetDataType())
	}
	if ft.GetSize() != 32 {
		t.Fatalf("expected size 32, got %d", ft.GetSize())
	}
}
func TestBuildTypedDataValueAckDomainChainID(t *testing.T) {
	chainID := math.NewHexOrDecimal256(1)
	td := &apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
		},
		PrimaryType: "Permit",
		Domain: apitypes.TypedDataDomain{
			Name:              "Test",
			Version:           "1",
			ChainId:           chainID,
			VerifyingContract: "0x0000000000000000000000000000000000000001",
		},
	}
	// chainId is the third field (index 2) in EIP712Domain above.
	value, err := buildTypedDataValueAck(td, []uint32{0, 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 32 {
		t.Fatalf("expected 32-byte uint256, got %d bytes", len(value))
	}
}

func TestEncodeTypedDataAtomicHexOrDecimal256(t *testing.T) {
	v := math.NewHexOrDecimal256(123456789)
	b, err := encodeTypedDataAtomic(v, "uint256")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(b))
	}
}

func TestEncodeTypedDataAtomicPermitValue(t *testing.T) {
	b, err := encodeTypedDataAtomic("1000000", "uint256")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(b))
	}
}

func TestBuildTypedDataStructAckPermit(t *testing.T) {
	td := &apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Permit": {
				{Name: "owner", Type: "address"},
				{Name: "spender", Type: "address"},
				{Name: "value", Type: "uint256"},
				{Name: "nonce", Type: "uint256"},
				{Name: "deadline", Type: "uint256"},
			},
		},
		PrimaryType: "Permit",
	}
	ack, err := buildTypedDataStructAck(td, "Permit")
	if err != nil {
		t.Fatal(err)
	}
	if len(ack.GetMembers()) != 5 {
		t.Fatalf("expected 5 members, got %d", len(ack.GetMembers()))
	}
}

func TestBuildTypedDataValueAckPermitOwner(t *testing.T) {
	td := &apitypes.TypedData{
		Types: apitypes.Types{
			"Permit": {
				{Name: "owner", Type: "address"},
				{Name: "spender", Type: "address"},
				{Name: "value", Type: "uint256"},
				{Name: "nonce", Type: "uint256"},
				{Name: "deadline", Type: "uint256"},
			},
		},
		PrimaryType: "Permit",
		Message: apitypes.TypedDataMessage{
			"owner":    "0x8180a5ca4e3b94045e05a9313777955f7518d757",
			"spender":  "0x0000000000000000000000000000000000000001",
			"value":    "1000000",
			"nonce":    "0",
			"deadline": "9999999999",
		},
	}
	value, err := buildTypedDataValueAck(td, []uint32{1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != 20 {
		t.Fatalf("expected 20-byte address, got %d bytes", len(value))
	}
}
