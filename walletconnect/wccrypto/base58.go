package wccrypto

// Minimal base58btc encoder (bitcoin alphabet), used exclusively to
// build did:key identifiers for the WalletConnect relay-auth JWT's
// issuer field. We don't need decoding or the many variants the full
// multibase spec defines — just forward encoding of small (<= 64 byte)
// inputs.
//
// The bitcoin alphabet excludes characters that are easy to confuse
// (0, O, I, l) so it's a stable fit for did:key identifiers.

const b58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// Base58BTCEncode returns the base58 representation of input using the
// bitcoin alphabet, with leading zero bytes encoded as leading '1'
// characters (matching the multibase 'z' / base58btc spec).
func Base58BTCEncode(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	// Count leading zero bytes — they encode to as many leading '1'.
	zeros := 0
	for zeros < len(input) && input[zeros] == 0 {
		zeros++
	}

	// Bit-for-bit, base58 expands by ~log(256)/log(58) ≈ 1.37.
	// Sizing to 2*len + zeros is a safe upper bound.
	size := (len(input)-zeros)*138/100 + 1
	buf := make([]byte, size)

	high := size - 1
	for i := zeros; i < len(input); i++ {
		carry := int(input[i])
		j := size - 1
		for ; j > high || carry != 0; j-- {
			carry += 256 * int(buf[j])
			buf[j] = byte(carry % 58)
			carry /= 58
			if j == 0 {
				break
			}
		}
		high = j
	}

	// Skip leading zeroes in buf.
	it := 0
	for it < size && buf[it] == 0 {
		it++
	}

	out := make([]byte, 0, zeros+size-it)
	for i := 0; i < zeros; i++ {
		out = append(out, '1')
	}
	for ; it < size; it++ {
		out = append(out, b58Alphabet[buf[it]])
	}
	return string(out)
}
