package wccrypto

import "testing"

// Reference vectors from https://datatracker.ietf.org/doc/html/draft-msporny-base58-03
// (appendix A) and https://en.bitcoin.it/wiki/Base58Check_encoding
func TestBase58BTCEncode_KnownVectors(t *testing.T) {
	tests := []struct {
		in   []byte
		want string
	}{
		{[]byte{}, ""},
		{[]byte("Hello World!"), "2NEpo7TZRRrLZSi2U"},
		{[]byte{0x00}, "1"},
		{[]byte{0x00, 0x00, 0x00}, "111"},
		{[]byte("The quick brown fox jumps over the lazy dog."),
			"USm3fpXnKG5EUBx2ndxBDMPVciP5hGey2Jh4NDv6gmeo1LkMeiKrLJUUBk6Z"},
	}
	for _, tc := range tests {
		got := Base58BTCEncode(tc.in)
		if got != tc.want {
			t.Errorf("Base58BTCEncode(%q) = %q, want %q", string(tc.in), got, tc.want)
		}
	}
}
