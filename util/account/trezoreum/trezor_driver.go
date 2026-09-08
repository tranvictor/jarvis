package trezoreum

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/golang/protobuf/proto"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
)

type CallMode int

const (
	CallModeRead      CallMode = 0
	CallModeWrite     CallMode = 1
	CallModeReadWrite CallMode = 2
)

// ErrTrezorPINNeeded is returned if opening the trezor requires a PIN code. In
// this case, the calling application should display a pinpad and send back the
// encoded passphrase.
var ErrTrezorPINNeeded = errors.New("trezor: pin needed")

// ErrTrezorPassphraseNeeded is returned if opening the trezor requires a passphrase
var ErrTrezorPassphraseNeeded = errors.New("trezor: passphrase needed")

// ErrCancelled is returned when the operator rejects the request on the Trezor
// or aborts Jarvis with Ctrl-C while the device is waiting for a button.
var ErrCancelled = errors.New("cancelled on Trezor")

// errTrezorReplyInvalidHeader is the error message returned by a Trezor data exchange
// if the device replies with a mismatching header. This usually means the device
// is in browser mode.
var errTrezorReplyInvalidHeader = errors.New("trezor: invalid reply header")

var errTrezorDeviceClosed = errors.New("trezor: device closed")

// trezorDriver implements the communication with a Trezor hardware wallet.
type TrezorDriver struct {
	device  io.ReadWriter // USB device connection to communicate through
	writeMu sync.Mutex    // serializes HID writes so Abort can run during a Read
}

// newTrezorDriver creates a new instance of a Trezor USB protocol driver.
func NewTrezorDriver() *TrezorDriver {
	return &TrezorDriver{}
}

func (self *TrezorDriver) SetDevice(device io.ReadWriter) {
	self.device = device
}

// Abort sends a Cancel message without waiting for a reply. The in-flight
// Exchange Read is expected to pick up the Failure the device sends back.
// Safe to call from another goroutine while Exchange is blocked on Read.
func (self *TrezorDriver) Abort() {
	if self.device == nil {
		return
	}
	_ = self.writeMessage(&trezor.Cancel{})
}

func (self *TrezorDriver) Exchange(req proto.Message, results ...proto.Message) (int, error) {
	return self.exchange(req, true, results...)
}

func (self *TrezorDriver) exchange(req proto.Message, announceButton bool, results ...proto.Message) (int, error) {
	if self.device == nil {
		return 0, errTrezorDeviceClosed
	}
	if err := self.writeMessage(req); err != nil {
		return 0, err
	}

	kind, reply, err := self.readMessage()
	if err != nil {
		return 0, err
	}

	if kind == uint16(trezor.MessageType_MessageType_Failure) {
		failure := new(trezor.Failure)
		if err := proto.Unmarshal(reply, failure); err != nil {
			return 0, err
		}
		return 0, failureError(failure)
	}
	if kind == uint16(trezor.MessageType_MessageType_ButtonRequest) {
		if announceButton {
			fmt.Printf("Confirm or reject on your Trezor…\n")
		}
		return self.exchange(&trezor.ButtonAck{}, false, results...)
	}
	if kind == uint16(trezor.MessageType_MessageType_Deprecated_PassphraseStateRequest) {
		return self.exchange(&trezor.Deprecated_PassphraseStateAck{}, announceButton, results...)
	}
	for i, res := range results {
		if trezor.Type(res) == kind {
			return i, proto.Unmarshal(reply, res)
		}
	}
	expected := make([]string, len(results))
	for i, res := range results {
		expected[i] = trezor.Name(trezor.Type(res))
	}

	return 0, fmt.Errorf("trezor: expected reply types %s, got %s", expected, trezor.Name(kind))
}

func (self *TrezorDriver) writeMessage(req proto.Message) error {
	if self.device == nil {
		return errTrezorDeviceClosed
	}
	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	payload := make([]byte, 8+len(data))
	copy(payload, []byte{0x23, 0x23})
	binary.BigEndian.PutUint16(payload[2:], trezor.Type(req))
	binary.BigEndian.PutUint32(payload[4:], uint32(len(data)))
	copy(payload[8:], data)

	self.writeMu.Lock()
	defer self.writeMu.Unlock()

	chunk := make([]byte, 64)
	chunk[0] = 0x3f
	for len(payload) > 0 {
		if len(payload) > 63 {
			copy(chunk[1:], payload[:63])
			payload = payload[63:]
		} else {
			copy(chunk[1:], payload)
			copy(chunk[1+len(payload):], make([]byte, 63-len(payload)))
			payload = nil
		}
		if _, err := self.device.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (self *TrezorDriver) readMessage() (uint16, []byte, error) {
	if self.device == nil {
		return 0, nil, errTrezorDeviceClosed
	}
	chunk := make([]byte, 64)
	var (
		kind  uint16
		reply []byte
	)
	for {
		if err := self.readReport(chunk); err != nil {
			return 0, nil, err
		}
		if chunk[0] != 0x3f || (len(reply) == 0 && (chunk[1] != 0x23 || chunk[2] != 0x23)) {
			return 0, nil, errTrezorReplyInvalidHeader
		}
		var payload []byte
		if len(reply) == 0 {
			kind = binary.BigEndian.Uint16(chunk[3:5])
			reply = make([]byte, 0, int(binary.BigEndian.Uint32(chunk[5:9])))
			payload = chunk[9:]
		} else {
			payload = chunk[1:]
		}
		if left := cap(reply) - len(reply); left > len(payload) {
			reply = append(reply, payload...)
		} else {
			reply = append(reply, payload[:left]...)
			break
		}
	}
	return kind, reply, nil
}

func (self *TrezorDriver) readReport(chunk []byte) error {
	n, err := self.device.Read(chunk)
	if err != nil {
		return err
	}
	if n == 0 {
		return io.ErrUnexpectedEOF
	}
	if n < len(chunk) {
		for i := n; i < len(chunk); i++ {
			chunk[i] = 0
		}
	}
	return nil
}

func failureError(failure *trezor.Failure) error {
	msg := failure.GetMessage()
	code := failure.GetCode()
	cancelled := code == trezor.Failure_Failure_ActionCancelled ||
		code == trezor.Failure_Failure_PinCancelled
	if !cancelled && msg != "" {
		cancelled = strings.Contains(strings.ToLower(msg), "cancel")
	}
	if cancelled {
		if msg == "" {
			msg = "cancelled"
		}
		return fmt.Errorf("%w (%s)", ErrCancelled, msg)
	}
	if msg == "" {
		msg = code.String()
	}
	return errors.New("trezor: " + msg)
}
