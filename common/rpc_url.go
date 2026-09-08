package common

import (
	"net"
	"path/filepath"
	"strings"
)

// CanonicalRPCURL makes a node URL safe for go-ethereum's rpc.Dial.
// Dial treats any string without a URI scheme as a Unix IPC path, so a
// stored host like rpc.example.com/path is tried as a socket and fails with
// "dial unix … no such file or directory". Public hosts get https://;
// loopback hosts get http://; real IPC paths are left alone.
func CanonicalRPCURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" || strings.Contains(u, "://") || isRPCIPCPath(u) {
		return u
	}
	if isRPCLoopbackHost(rpcURLHost(u)) {
		return "http://" + u
	}
	return "https://" + u
}

func isRPCIPCPath(u string) bool {
	if strings.HasSuffix(strings.ToLower(u), ".ipc") {
		return true
	}
	if filepath.IsAbs(u) || strings.HasPrefix(u, "./") || strings.HasPrefix(u, "../") {
		return true
	}
	if len(u) >= 3 && u[1] == ':' && (u[2] == '\\' || u[2] == '/') {
		return true
	}
	return strings.HasPrefix(u, `\\.\pipe\`) || strings.HasPrefix(u, `//./pipe/`)
}

func rpcURLHost(u string) string {
	host := u
	if i := strings.Index(u, "/"); i >= 0 {
		host = u[:i]
	}
	if strings.HasPrefix(host, "[") {
		if j := strings.Index(host, "]"); j >= 0 {
			return host[1:j]
		}
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func isRPCLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
