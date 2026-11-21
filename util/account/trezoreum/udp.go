package trezoreum

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/tranvictor/jarvis/util/account/usb"
)

// UDPEnv names an address such as 127.0.0.1:21324. When set, Init talks to
// the Trezor emulator on that UDP port instead of looking for USB hardware.
const UDPEnv = "JARVIS_TREZOR_UDP"

type udpDevice struct {
	conn *net.UDPConn
}

func openUDP(addr string) (usb.Device, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	d := &udpDevice{conn: conn}
	if _, err := conn.Write([]byte("PINGPING")); err != nil {
		conn.Close()
		return nil, err
	}
	buf := make([]byte, 64)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil || (string(buf[:n]) != "PONGPONG" && string(buf[:n]) != "PONG") {
		conn.Close()
		return nil, fmt.Errorf("no Trezor emulator answered on %s", addr)
	}
	return d, nil
}

func (d *udpDevice) Close() error { return d.conn.Close() }

func (d *udpDevice) Write(b []byte) (int, error) { return d.conn.Write(b) }

func (d *udpDevice) Read(b []byte) (int, error) {
	_ = d.conn.SetReadDeadline(time.Time{})
	return d.conn.Read(b)
}

func (d *udpDevice) ReadTimeout(b []byte, timeout time.Duration) (int, error) {
	_ = d.conn.SetReadDeadline(time.Now().Add(timeout))
	n, err := d.conn.Read(b)
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return 0, nil
	}
	return n, err
}

func emulatorAddr() string { return os.Getenv(UDPEnv) }
