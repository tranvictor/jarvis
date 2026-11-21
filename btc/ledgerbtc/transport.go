package ledgerbtc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/tranvictor/jarvis/util/account/ledgereum"
	"github.com/tranvictor/jarvis/util/account/usb"
)

// SpeculosEnv names the APDU TCP address (for example 127.0.0.1:9999) of a
// Speculos emulator. When set, the client talks to it instead of USB.
const SpeculosEnv = "JARVIS_LEDGER_SPECULOS"

// ErrNoDevice means no Ledger could be found or opened.
var ErrNoDevice = errors.New("Ledger device is not found")

// Transport sends one APDU and returns the data and status word. Status
// words other than 0x9000 are not errors at this level.
type Transport interface {
	Exchange(cla, ins, p1, p2 byte, data []byte) (resp []byte, sw uint16, err error)
	Close() error
}

// Open connects to a Speculos emulator when SpeculosEnv is set, otherwise to
// the first Ledger on USB.
func Open() (Transport, error) {
	if addr := os.Getenv(SpeculosEnv); addr != "" {
		return OpenSpeculos(addr)
	}
	return OpenHID()
}

type speculos struct{ conn net.Conn }

func OpenSpeculos(addr string) (Transport, error) {
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("%w: no Speculos on %s", ErrNoDevice, addr)
	}
	return &speculos{conn: c}, nil
}

func (s *speculos) Close() error { return s.conn.Close() }

func (s *speculos) Exchange(cla, ins, p1, p2 byte, data []byte) ([]byte, uint16, error) {
	apdu := append([]byte{cla, ins, p1, p2, byte(len(data))}, data...)
	frame := make([]byte, 4, 4+len(apdu))
	binary.BigEndian.PutUint32(frame, uint32(len(apdu)))
	if _, err := s.conn.Write(append(frame, apdu...)); err != nil {
		return nil, 0, err
	}
	var hdr [4]byte
	if _, err := io.ReadFull(s.conn, hdr[:]); err != nil {
		return nil, 0, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	body := make([]byte, int(n)+2)
	if _, err := io.ReadFull(s.conn, body); err != nil {
		return nil, 0, err
	}
	return body[:n], binary.BigEndian.Uint16(body[n:]), nil
}

type hid struct{ dev usb.Device }

// OpenHID opens the first Ledger found on USB.
func OpenHID() (Transport, error) {
	infos, err := usb.Enumerate(ledgereum.LEDGER_VENDOR_ID, 0)
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		for _, id := range ledgereum.LEDGER_PRODUCT_IDS {
			if info.ProductID == id && (info.UsagePage == ledgereum.LEDGER_USAGE_ID || info.Interface == ledgereum.LEDGER_ENDPOINT_ID) {
				dev, err := info.Open()
				if err != nil {
					return nil, err
				}
				return &hid{dev: dev}, nil
			}
		}
	}
	return nil, ErrNoDevice
}

func (h *hid) Close() error { return h.dev.Close() }

// Exchange frames the APDU in 64-byte HID reports: channel 0x0101, tag 0x05,
// a 16-bit sequence number, and a 16-bit length in the first report.
func (h *hid) Exchange(cla, ins, p1, p2 byte, data []byte) ([]byte, uint16, error) {
	apdu := make([]byte, 2, 7+len(data))
	binary.BigEndian.PutUint16(apdu, uint16(5+len(data)))
	apdu = append(apdu, cla, ins, p1, p2, byte(len(data)))
	apdu = append(apdu, data...)

	header := []byte{0x01, 0x01, 0x05, 0x00, 0x00}
	space := 64 - len(header)
	for i := 0; len(apdu) > 0; i++ {
		chunk := append([]byte(nil), header...)
		binary.BigEndian.PutUint16(chunk[3:], uint16(i))
		n := len(apdu)
		if n > space {
			n = space
		}
		chunk = append(chunk, apdu[:n]...)
		apdu = apdu[n:]
		chunk = append(chunk, make([]byte, 64-len(chunk))...)
		if _, err := h.dev.Write(chunk); err != nil {
			return nil, 0, err
		}
	}

	var reply []byte
	var want int
	chunk := make([]byte, 64)
	for first := true; ; first = false {
		if _, err := io.ReadFull(h.dev, chunk); err != nil {
			return nil, 0, err
		}
		if chunk[0] != 0x01 || chunk[1] != 0x01 || chunk[2] != 0x05 {
			return nil, 0, errors.New("ledger: invalid reply header")
		}
		payload := chunk[5:]
		if first {
			want = int(binary.BigEndian.Uint16(chunk[5:7]))
			payload = chunk[7:]
		}
		if left := want - len(reply); left > len(payload) {
			reply = append(reply, payload...)
		} else {
			reply = append(reply, payload[:left]...)
			break
		}
	}
	if len(reply) < 2 {
		return nil, 0, errors.New("ledger: reply too short")
	}
	return reply[:len(reply)-2], binary.BigEndian.Uint16(reply[len(reply)-2:]), nil
}
