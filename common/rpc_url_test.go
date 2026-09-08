package common

import "testing"

func TestCanonicalRPCURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"  rpc.example.com/rpc/KEY  ", "https://rpc.example.com/rpc/KEY"},
		{"rpc.example.com", "https://rpc.example.com"},
		{"rpc.example.com:443/path", "https://rpc.example.com:443/path"},
		{"https://rpc.example.com/path", "https://rpc.example.com/path"},
		{"http://rpc.example.com", "http://rpc.example.com"},
		{"wss://rpc.example.com/ws", "wss://rpc.example.com/ws"},
		{"localhost:8545", "http://localhost:8545"},
		{"LOCALHOST/rpc", "http://LOCALHOST/rpc"},
		{"127.0.0.1:8545", "http://127.0.0.1:8545"},
		{"[::1]:8545", "http://[::1]:8545"},
		{"/var/run/geth.ipc", "/var/run/geth.ipc"},
		{"./geth.ipc", "./geth.ipc"},
		{"geth.ipc", "geth.ipc"},
		{`\\.\pipe\geth.ipc`, `\\.\pipe\geth.ipc`},
		{`C:\geth\geth.ipc`, `C:\geth\geth.ipc`},
	}
	for _, c := range cases {
		got := CanonicalRPCURL(c.in)
		if got != c.want {
			t.Errorf("CanonicalRPCURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
