package trezoreum

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/tranvictor/jarvis/util/account/trezoreum/trezor"
)

type fakeTrezor struct {
	mu       sync.Mutex
	incoming []byte
	kind     uint16
	want     int
	replies  chan []byte
	onMsg    func(kind uint16)
}

func newFakeTrezor(onMsg func(kind uint16)) *fakeTrezor {
	return &fakeTrezor{
		replies: make(chan []byte, 32),
		onMsg:   onMsg,
	}
}

func (f *fakeTrezor) reply(msg proto.Message) {
	for _, chunk := range encodeTrezorHID(msg) {
		f.replies <- chunk
	}
}

func (f *fakeTrezor) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(b) < 1 || b[0] != 0x3f {
		return 0, errors.New("bad hid magic")
	}
	payload := b[1:]
	if len(f.incoming) == 0 {
		if len(payload) < 8 || payload[0] != 0x23 || payload[1] != 0x23 {
			return 0, errors.New("bad trezor header")
		}
		f.kind = binary.BigEndian.Uint16(payload[2:4])
		f.want = int(binary.BigEndian.Uint32(payload[4:8]))
		f.incoming = append([]byte(nil), payload[8:]...)
	} else {
		f.incoming = append(f.incoming, payload...)
	}
	if len(f.incoming) >= f.want {
		f.incoming = f.incoming[:f.want]
		kind := f.kind
		f.incoming = nil
		f.want = 0
		onMsg := f.onMsg
		if onMsg != nil {
			// Drop the lock so onMsg can reply without deadlock.
			f.mu.Unlock()
			onMsg(kind)
			f.mu.Lock()
		}
	}
	return len(b), nil
}

func (f *fakeTrezor) Read(b []byte) (int, error) {
	chunk := <-f.replies
	if chunk == nil {
		return 0, io.EOF
	}
	n := copy(b, chunk)
	return n, nil
}

func encodeTrezorHID(msg proto.Message) [][]byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err)
	}
	payload := make([]byte, 8+len(data))
	payload[0], payload[1] = 0x23, 0x23
	binary.BigEndian.PutUint16(payload[2:], trezor.Type(msg))
	binary.BigEndian.PutUint32(payload[4:], uint32(len(data)))
	copy(payload[8:], data)

	var chunks [][]byte
	for len(payload) > 0 {
		chunk := make([]byte, 64)
		chunk[0] = 0x3f
		n := 63
		if len(payload) < n {
			n = len(payload)
		}
		copy(chunk[1:], payload[:n])
		payload = payload[n:]
		chunks = append(chunks, chunk)
	}
	return chunks
}

func cancelledFailure() *trezor.Failure {
	code := trezor.Failure_Failure_ActionCancelled
	msg := "Cancelled"
	return &trezor.Failure{Code: &code, Message: &msg}
}

func TestExchangeDeviceCancelAfterButton(t *testing.T) {
	dev := newFakeTrezor(nil)
	dev.onMsg = func(kind uint16) {
		switch kind {
		case uint16(trezor.MessageType_MessageType_Ping):
			dev.reply(&trezor.ButtonRequest{})
		case uint16(trezor.MessageType_MessageType_ButtonAck):
			dev.reply(cancelledFailure())
		}
	}
	d := NewTrezorDriver()
	d.SetDevice(dev)

	_, err := d.Exchange(&trezor.Ping{}, new(trezor.Success))
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("got %v, want ErrCancelled", err)
	}
}

func TestAbortUnblocksButtonWait(t *testing.T) {
	dev := newFakeTrezor(nil)
	dev.onMsg = func(kind uint16) {
		switch kind {
		case uint16(trezor.MessageType_MessageType_Ping):
			dev.reply(&trezor.ButtonRequest{})
		case uint16(trezor.MessageType_MessageType_ButtonAck):
			// Wait for Cancel, as a real device does after ButtonAck.
		case uint16(trezor.MessageType_MessageType_Cancel):
			dev.reply(cancelledFailure())
		}
	}
	d := NewTrezorDriver()
	d.SetDevice(dev)

	done := make(chan error, 1)
	go func() {
		_, err := d.Exchange(&trezor.Ping{}, new(trezor.Success))
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	d.Abort()

	select {
	case err := <-done:
		if !errors.Is(err, ErrCancelled) {
			t.Fatalf("got %v, want ErrCancelled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Exchange did not return after Abort")
	}
}

func TestFailureErrorEmptyCancelledMessage(t *testing.T) {
	code := trezor.Failure_Failure_ActionCancelled
	err := failureError(&trezor.Failure{Code: &code})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("got %v, want ErrCancelled", err)
	}
}
