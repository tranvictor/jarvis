package common

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// MultiSendMethodName is the single method exposed by both MultiSend and
// MultiSendCallOnly.
const MultiSendMethodName = "multiSend"

// MultiSendSelector is the 4-byte selector of multiSend(bytes), i.e.
// keccak256("multiSend(bytes)")[:4]. Hardcoded rather than derived so that
// callers can compare against raw calldata without building an ABI.
var MultiSendSelector = []byte{0x8d, 0x80, 0xff, 0x0a}

// multiSendCallHeaderLen is the fixed-size prefix of every packed entry:
// operation(1) + to(20) + value(32) + dataLength(32).
const multiSendCallHeaderLen = 1 + 20 + 32 + 32

// MultiSendCall is one entry of a MultiSend batch.
//
// Operation is 0 for CALL and 1 for DELEGATECALL. MultiSendCallOnly reverts
// on any entry with Operation != 0, which is why jarvis only ever produces
// 0 — the Safe transaction-builder format has no per-entry operation field.
type MultiSendCall struct {
	Operation uint8
	To        common.Address
	Value     *big.Int
	Data      []byte
}

// EncodeMultiSendPayload builds the `transactions` argument of
// multiSend(bytes). The layout is Safe's own hand-packed (i.e. NOT
// abi-encoded) concatenation, per entry:
//
//	operation  uint8    1 byte
//	to         address  20 bytes
//	value      uint256  32 bytes, big-endian
//	dataLength uint256  32 bytes, big-endian
//	data       bytes    dataLength bytes, unpadded
func EncodeMultiSendPayload(calls []MultiSendCall) []byte {
	total := 0
	for _, c := range calls {
		total += multiSendCallHeaderLen + len(c.Data)
	}

	out := make([]byte, 0, total)
	for _, c := range calls {
		value := c.Value
		if value == nil {
			value = big.NewInt(0)
		}

		out = append(out, c.Operation)
		out = append(out, c.To.Bytes()...)
		out = append(out, common.LeftPadBytes(value.Bytes(), 32)...)
		out = append(out, common.LeftPadBytes(big.NewInt(int64(len(c.Data))).Bytes(), 32)...)
		out = append(out, c.Data...)
	}
	return out
}

// DecodeMultiSendPayload is the inverse of EncodeMultiSendPayload. It is
// deliberately strict: a truncated entry, a length prefix that overruns the
// buffer, or trailing bytes past the last entry are all errors rather than a
// best-effort partial decode, because the result is shown to an operator who
// is about to sign it.
func DecodeMultiSendPayload(payload []byte) ([]MultiSendCall, error) {
	var calls []MultiSendCall

	for offset := 0; offset < len(payload); {
		remaining := len(payload) - offset
		if remaining < multiSendCallHeaderLen {
			return nil, fmt.Errorf(
				"truncated multiSend entry %d at offset %d: need %d header bytes, got %d",
				len(calls), offset, multiSendCallHeaderLen, remaining,
			)
		}

		c := MultiSendCall{Operation: payload[offset]}
		offset++
		c.To = common.BytesToAddress(payload[offset : offset+20])
		offset += 20
		c.Value = new(big.Int).SetBytes(payload[offset : offset+32])
		offset += 32

		lengthWord := payload[offset : offset+32]
		offset += 32
		// A data length that doesn't fit in 64 bits can't possibly be
		// satisfied by the buffer; reject it before it overflows.
		for _, b := range lengthWord[:24] {
			if b != 0 {
				return nil, fmt.Errorf(
					"multiSend entry %d declares an absurd data length", len(calls),
				)
			}
		}
		dataLen := binary.BigEndian.Uint64(lengthWord[24:])
		if dataLen > uint64(len(payload)-offset) {
			return nil, fmt.Errorf(
				"multiSend entry %d declares %d data bytes but only %d remain",
				len(calls), dataLen, len(payload)-offset,
			)
		}

		c.Data = make([]byte, dataLen)
		copy(c.Data, payload[offset:offset+int(dataLen)])
		offset += int(dataLen)

		calls = append(calls, c)
	}

	if len(calls) == 0 {
		return nil, fmt.Errorf("multiSend payload is empty")
	}
	return calls, nil
}

// PackMultiSend returns the full calldata for a MultiSend batch, i.e. the
// multiSend(bytes) selector followed by the abi-encoded packed payload.
func PackMultiSend(calls []MultiSendCall) ([]byte, error) {
	if len(calls) == 0 {
		return nil, fmt.Errorf("can't pack an empty MultiSend batch")
	}
	return GetMultiSendABI().Pack(MultiSendMethodName, EncodeMultiSendPayload(calls))
}

// IsMultiSendCallData reports whether data starts with the multiSend(bytes)
// selector.
func IsMultiSendCallData(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	for i, b := range MultiSendSelector {
		if data[i] != b {
			return false
		}
	}
	return true
}
