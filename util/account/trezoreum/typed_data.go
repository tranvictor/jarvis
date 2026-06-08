package trezoreum

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
)

var (
	eip712ArrayTypeRe = regexp.MustCompile(`^(.+)\[(\d*)\]$`)
	eip712IntSizeRe   = regexp.MustCompile(`(\d+)$`)
)

func (self *Trezoreum) SignTypedData(
	path accounts.DerivationPath,
	td *apitypes.TypedData,
) ([]byte, error) {
	if td == nil {
		return nil, fmt.Errorf("nil typed data")
	}
	if td.PrimaryType == "" {
		return nil, fmt.Errorf("typed data missing primaryType")
	}

	primaryType := td.PrimaryType
	mmCompat := true
	req := &trezor.EthereumSignTypedData{
		AddressN:         path,
		PrimaryType:      &primaryType,
		MetamaskV4Compat: &mmCompat,
	}

	structReq := new(trezor.EthereumTypedDataStructRequest)
	valueReq := new(trezor.EthereumTypedDataValueRequest)
	sigResp := new(trezor.EthereumTypedDataSignature)

	started := false
	var signErr error
	defer func() {
		if started && signErr != nil {
			self.tryCancelSigning()
		}
	}()

	respIdx, err := self.trezorExchange(req, structReq, valueReq, sigResp)
	if err != nil {
		signErr = err
		return nil, err
	}
	started = true

	for {
		switch respIdx {
		case 0:
			ack, err := buildTypedDataStructAck(td, structReq.GetName())
			if err != nil {
				signErr = err
				return nil, err
			}
			respIdx, err = self.trezorExchange(ack, structReq, valueReq, sigResp)
		case 1:
			value, err := buildTypedDataValueAck(td, valueReq.GetMemberPath())
			if err != nil {
				signErr = err
				return nil, err
			}
			respIdx, err = self.trezorExchange(
				&trezor.EthereumTypedDataValueAck{Value: value},
				structReq, valueReq, sigResp,
			)
		case 2:
			sig, err := normalizeTrezorSignature(sigResp.GetSignature())
			signErr = err
			return sig, err
		default:
			signErr = fmt.Errorf("unexpected Trezor EIP-712 response index %d", respIdx)
			return nil, signErr
		}
		if err != nil {
			signErr = err
			return nil, err
		}
	}
}

func (self *Trezoreum) tryCancelSigning() {
	_, _ = self.trezorExchange(
		&trezor.Cancel{},
		new(trezor.Success),
		new(trezor.Failure),
	)
}

func normalizeTrezorSignature(sig []byte) ([]byte, error) {
	if len(sig) != 65 {
		return nil, fmt.Errorf("trezor EIP-712 signature has length %d, want 65", len(sig))
	}
	out := make([]byte, 65)
	copy(out, sig)
	if out[64] < 27 {
		out[64] += 27
	}
	return out, nil
}

func buildTypedDataStructAck(
	td *apitypes.TypedData,
	structName string,
) (*trezor.EthereumTypedDataStructAck, error) {
	fields, ok := td.Types[structName]
	if !ok {
		return nil, fmt.Errorf("typed data missing struct %q", structName)
	}
	members := make([]*trezor.EthereumTypedDataStructAck_EthereumStructMember, 0, len(fields))
	for _, field := range fields {
		ft, err := trezorFieldType(field.Type, td.Types)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", field.Name, err)
		}
		name := field.Name
		members = append(members, &trezor.EthereumTypedDataStructAck_EthereumStructMember{
			Type: ft,
			Name: &name,
		})
	}
	return &trezor.EthereumTypedDataStructAck{Members: members}, nil
}

func buildTypedDataValueAck(td *apitypes.TypedData, memberPath []uint32) ([]byte, error) {
	if len(memberPath) == 0 {
		return nil, fmt.Errorf("empty member path")
	}
	root := memberPath[0]
	var (
		typeName string
		data     interface{}
	)
	switch root {
	case 0:
		typeName = "EIP712Domain"
		data = td.Domain.Map()
	case 1:
		typeName = td.PrimaryType
		data = td.Message
	default:
		return nil, fmt.Errorf("root index %d out of range", root)
	}

	for _, index := range memberPath[1:] {
		switch cur := data.(type) {
		case map[string]interface{}:
			fields, ok := td.Types[typeName]
			if !ok {
				return nil, fmt.Errorf("unknown struct %q", typeName)
			}
			if int(index) >= len(fields) {
				return nil, fmt.Errorf("member index %d out of range for %q", index, typeName)
			}
			field := fields[index]
			typeName = field.Type
			var found bool
			data, found = cur[field.Name]
			if !found {
				return nil, fmt.Errorf("missing field %q in %q", field.Name, typeName)
			}
		case []interface{}:
			if int(index) >= len(cur) {
				return nil, fmt.Errorf("array index %d out of range", index)
			}
			typeName = arrayElementType(typeName)
			data = cur[index]
		default:
			return nil, fmt.Errorf("cannot traverse %T at path %v", data, memberPath)
		}
	}

	if arr, ok := data.([]interface{}); ok {
		if len(arr) > 0xffff {
			return nil, fmt.Errorf("array too large for Trezor (%d elements)", len(arr))
		}
		buf := make([]byte, 2)
		buf[0] = byte(len(arr) >> 8)
		buf[1] = byte(len(arr))
		return buf, nil
	}

	return encodeTypedDataAtomic(data, typeName)
}

func trezorFieldType(
	typeName string,
	types map[string][]apitypes.Type,
) (*trezor.EthereumTypedDataStructAck_EthereumFieldType, error) {
	ft := &trezor.EthereumTypedDataStructAck_EthereumFieldType{}

	if m := eip712ArrayTypeRe.FindStringSubmatch(typeName); m != nil {
		elemType := m[1]
		entry, err := trezorFieldType(elemType, types)
		if err != nil {
			return nil, err
		}
		if entry.GetDataType() == trezor.EthereumTypedDataStructAck_ARRAY {
			return nil, fmt.Errorf("nested arrays are not supported")
		}
		dt := trezor.EthereumTypedDataStructAck_ARRAY
		ft.DataType = &dt
		ft.EntryType = entry
		if m[2] != "" {
			n, err := strconv.ParseUint(m[2], 10, 32)
			if err != nil {
				return nil, err
			}
			size := uint32(n)
			ft.Size = &size
		}
		return ft, nil
	}

	switch {
	case strings.HasPrefix(typeName, "uint"):
		dt := trezor.EthereumTypedDataStructAck_UINT
		ft.DataType = &dt
		size := uint32(parseIntTypeBytes(typeName))
		ft.Size = &size
	case strings.HasPrefix(typeName, "int"):
		dt := trezor.EthereumTypedDataStructAck_INT
		ft.DataType = &dt
		size := uint32(parseIntTypeBytes(typeName))
		ft.Size = &size
	case strings.HasPrefix(typeName, "bytes"):
		dt := trezor.EthereumTypedDataStructAck_BYTES
		ft.DataType = &dt
		if typeName != "bytes" {
			n, err := strconv.ParseUint(strings.TrimPrefix(typeName, "bytes"), 10, 32)
			if err != nil {
				return nil, err
			}
			size := uint32(n)
			ft.Size = &size
		}
	case typeName == "string":
		dt := trezor.EthereumTypedDataStructAck_STRING
		ft.DataType = &dt
	case typeName == "bool":
		dt := trezor.EthereumTypedDataStructAck_BOOL
		ft.DataType = &dt
	case typeName == "address":
		dt := trezor.EthereumTypedDataStructAck_ADDRESS
		ft.DataType = &dt
	default:
		fields, ok := types[typeName]
		if !ok {
			return nil, fmt.Errorf("unsupported type %q", typeName)
		}
		dt := trezor.EthereumTypedDataStructAck_STRUCT
		ft.DataType = &dt
		size := uint32(len(fields))
		ft.Size = &size
		ft.StructName = &typeName
	}
	return ft, nil
}

func parseIntTypeBytes(typeName string) int {
	m := eip712IntSizeRe.FindString(typeName)
	if m == "" {
		return 32
	}
	n, _ := strconv.Atoi(m)
	return n / 8
}

func arrayElementType(typeName string) string {
	if m := eip712ArrayTypeRe.FindStringSubmatch(typeName); m != nil {
		return m[1]
	}
	return typeName
}

func encodeTypedDataAtomic(value interface{}, typeName string) ([]byte, error) {
	switch {
	case strings.HasPrefix(typeName, "bytes"):
		b, err := asBytes(value)
		if err != nil {
			return nil, err
		}
		if typeName != "bytes" {
			n, err := strconv.Atoi(strings.TrimPrefix(typeName, "bytes"))
			if err != nil {
				return nil, err
			}
			if len(b) != n {
				return nil, fmt.Errorf("%q has length %d, want %d", typeName, len(b), n)
			}
		}
		return b, nil
	case typeName == "string":
		s, err := asString(value)
		if err != nil {
			return nil, err
		}
		return []byte(s), nil
	case strings.HasPrefix(typeName, "uint"):
		n, err := asBigInt(value)
		if err != nil {
			return nil, err
		}
		byteLen := parseIntTypeBytes(typeName)
		return padBigInt(n, byteLen, false)
	case strings.HasPrefix(typeName, "int"):
		n, err := asBigInt(value)
		if err != nil {
			return nil, err
		}
		byteLen := parseIntTypeBytes(typeName)
		return padBigInt(n, byteLen, true)
	case typeName == "bool":
		b, err := asBool(value)
		if err != nil {
			return nil, err
		}
		if b {
			return []byte{1}, nil
		}
		return []byte{0}, nil
	case typeName == "address":
		return asAddressBytes(value)
	default:
		return nil, fmt.Errorf("unsupported atomic type %q", typeName)
	}
}

func asBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	default:
		return false, fmt.Errorf("invalid bool value %v", value)
	}
}

func asAddressBytes(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case string:
		if !common.IsHexAddress(v) {
			return nil, fmt.Errorf("invalid address %q", v)
		}
		return common.HexToAddress(v).Bytes(), nil
	case common.Address:
		return v.Bytes(), nil
	case []byte:
		if len(v) != 20 {
			return nil, fmt.Errorf("invalid address bytes length %d", len(v))
		}
		return v, nil
	default:
		if b, err := asBytes(value); err == nil && len(b) == 20 {
			return b, nil
		}
		return nil, fmt.Errorf("expected address, got %T", value)
	}
}

func asBytes(value interface{}) ([]byte, error) {
	if b, err := parseByteSlice(value); err == nil {
		return b, nil
	}
	s, err := asString(value)
	if err != nil {
		return nil, err
	}
	return decodeHexBytes(s)
}

func parseByteSlice(value interface{}) ([]byte, error) {
	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Array && val.Type().Elem().Kind() == reflect.Uint8 {
		v := reflect.MakeSlice(reflect.TypeOf([]byte{}), val.Len(), val.Len())
		reflect.Copy(v, val)
		return v.Bytes(), nil
	}
	switch v := value.(type) {
	case []byte:
		return v, nil
	case hexutil.Bytes:
		return []byte(v), nil
	default:
		return nil, fmt.Errorf("not a byte slice")
	}
}

func asString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	default:
		return "", fmt.Errorf("expected string, got %T", value)
	}
}

func asBigInt(value interface{}) (*big.Int, error) {
	switch v := value.(type) {
	case *math.HexOrDecimal256:
		if v == nil {
			return big.NewInt(0), nil
		}
		return (*big.Int)(v), nil
	case math.HexOrDecimal256:
		return (*big.Int)(&v), nil
	case *math.HexOrDecimal64:
		if v == nil {
			return big.NewInt(0), nil
		}
		return new(big.Int).SetUint64(uint64(*v)), nil
	case math.HexOrDecimal64:
		return new(big.Int).SetUint64(uint64(v)), nil
	case *big.Int:
		if v == nil {
			return big.NewInt(0), nil
		}
		return v, nil
	case string:
		if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
			n, ok := new(big.Int).SetString(v[2:], 16)
			if !ok {
				return nil, fmt.Errorf("invalid hex integer %q", v)
			}
			return n, nil
		}
		n, ok := new(big.Int).SetString(v, 10)
		if !ok {
			return nil, fmt.Errorf("invalid integer %q", v)
		}
		return n, nil
	case json.Number:
		n, ok := new(big.Int).SetString(v.String(), 10)
		if !ok {
			return nil, fmt.Errorf("invalid json.Number %q", v)
		}
		return n, nil
	case float64:
		if float64(int64(v)) != v {
			return nil, fmt.Errorf("invalid float value %v for integer field", v)
		}
		return big.NewInt(int64(v)), nil
	default:
		return nil, fmt.Errorf("expected integer, got %T", value)
	}
}

func padBigInt(n *big.Int, byteLen int, signed bool) ([]byte, error) {
	if byteLen <= 0 {
		byteLen = 32
	}
	out := make([]byte, byteLen)
	b := n.Bytes()
	if len(b) > byteLen {
		return nil, fmt.Errorf("integer %s does not fit in %d bytes", n, byteLen)
	}
	copy(out[byteLen-len(b):], b)
	if signed && n.Sign() < 0 {
		for i := 0; i < byteLen-len(b); i++ {
			out[i] = 0xff
		}
	}
	return out, nil
}

func decodeHexBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	if len(s)%2 != 0 {
		s = "0" + s
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(out); i++ {
		b, err := strconv.ParseUint(s[i*2:i*2+2], 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid hex %q: %w", s, err)
		}
		out[i] = byte(b)
	}
	return out, nil
}
