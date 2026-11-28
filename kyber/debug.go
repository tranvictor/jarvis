package kyber

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
)

// Debug writes go to ~/.jarvis/debug.out when --debug is on.
//
// Ways this can fail, and what the code does about each:
//   - The flag is off: the file is never opened and nothing is written.
//   - The home directory is unknown: the caller gets the error and the
//     portfolio stops before any request.
//   - ~/.jarvis is missing: it is created, then the file is opened for append.
//   - Two responses arrive together: one write finishes before the next starts.
//   - A call fails before a body exists: the URL is still recorded, with the
//     error on the following line.
//   - The body is not valid JSON, or the status is not 2xx: the bytes that
//     arrived are stored unchanged.

var (
	debugMu   sync.Mutex
	debugFile *os.File
)

// HomeDebugPath is ~/.jarvis/debug.out under the current user's home.
func HomeDebugPath() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("jarvis home directory is unknown")
	}
	return filepath.Join(u.HomeDir, ".jarvis", "debug.out"), nil
}

// EnableDebug appends later records to path. An empty path turns it off.
// The directory is created when path is set.
func EnableDebug(path string) error {
	debugMu.Lock()
	defer debugMu.Unlock()
	if debugFile != nil {
		_ = debugFile.Close()
		debugFile = nil
	}
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	debugFile = f
	return nil
}

func debugAppend(header string, body []byte) {
	debugMu.Lock()
	defer debugMu.Unlock()
	if debugFile == nil {
		return
	}
	_, _ = debugFile.WriteString(header)
	_, _ = debugFile.WriteString("\n")
	if len(body) > 0 {
		_, _ = debugFile.Write(body)
		if body[len(body)-1] != '\n' {
			_, _ = debugFile.WriteString("\n")
		}
	}
	_, _ = debugFile.WriteString("\n")
	_ = debugFile.Sync()
}
