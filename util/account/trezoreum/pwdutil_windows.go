// only build on windows to fix recent bug in
// golang.org/x/crypto/ssh/terminal

// +build windows

package trezoreum

import (
	"golang.org/x/sys/windows"
	"io"
	"os"
	"runtime"
)

// readPasswordLine reads from reader until it finds \n or io.EOF.
// The slice returned does not include the \n.
// readPasswordLine also ignores any \r it finds.
// Windows uses \r as end of line. So, on Windows, readPasswordLine
// reads until it finds \r and ignores any \n it finds during processing.
func readPasswordLine(reader io.Reader) ([]byte, error) {
	var buf [1]byte
	var ret []byte

	for {
		n, err := reader.Read(buf[:])
		if n > 0 {
			switch buf[0] {
			case '\b':
				if len(ret) > 0 {
					ret = ret[:len(ret)-1]
				}
			case '\n':
				if runtime.GOOS != "windows" {
					return ret, nil
				}
				// otherwise ignore \n
			case '\r':
				if runtime.GOOS == "windows" {
					return ret, nil
				}
				// otherwise ignore \r
			default:
				ret = append(ret, buf[0])
			}
			continue
		}
		if err != nil {
			if err == io.EOF && len(ret) > 0 {
				return ret, nil
			}
			return ret, err
		}
	}
}

// ReadPassword reads a line of input from a terminal without local echo.  This
// is commonly used for inputting passwords and other sensitive data. The slice
// returned does not include the \n.
func ReadPassword(fd int) ([]byte, error) {
	var st uint32
	if err := windows.GetConsoleMode(windows.Handle(fd), &st); err != nil {
		return nil, err
	}
	old := st

	st &^= (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT)
	st |= (windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_PROCESSED_INPUT)
	if err := windows.SetConsoleMode(windows.Handle(fd), st); err != nil {
		return nil, err
	}

	defer windows.SetConsoleMode(windows.Handle(fd), old)

	var h windows.Handle
	p, _ := windows.GetCurrentProcess()
	if err := windows.DuplicateHandle(p, windows.Handle(fd), p, &h, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}

	f := os.NewFile(uintptr(h), "stdin")
	defer f.Close()
	return readPasswordLine(f)
}
