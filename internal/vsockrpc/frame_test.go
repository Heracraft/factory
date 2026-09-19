package vsockrpc

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
)

func pipe(t *testing.T) (a, b net.Conn) {
	t.Helper()
	a, b = net.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	return a, b
}

func TestConnRoundTrip(t *testing.T) {
	a, b := pipe(t)
	ca, cb := NewConn(a), NewConn(b)

	want := &guestdv1.Envelope{
		RequestId: "req-1",
		Body: &guestdv1.Envelope_Request{Request: &guestdv1.Request{
			Req: &guestdv1.Request_Switch{Switch: &guestdv1.Switch{SystemClosure: "/nix/store/abc-system"}},
		}},
	}
	go func() {
		if err := ca.Send(want); err != nil {
			t.Errorf("send: %v", err)
		}
	}()
	got, err := cb.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	if got.GetRequestId() != "req-1" {
		t.Errorf("request_id = %q, want req-1", got.GetRequestId())
	}
	if got.GetRequest().GetSwitch().GetSystemClosure() != "/nix/store/abc-system" {
		t.Errorf("closure did not survive the round trip: %+v", got.GetRequest())
	}
}

func TestConnFramesAreIndependent(t *testing.T) {
	a, b := pipe(t)
	ca, cb := NewConn(a), NewConn(b)

	go func() {
		for i := 0; i < 5; i++ {
			env := &guestdv1.Envelope{RequestId: string(rune('a' + i))}
			if err := ca.Send(env); err != nil {
				t.Errorf("send %d: %v", i, err)
				return
			}
		}
	}()
	for i := 0; i < 5; i++ {
		got, err := cb.Recv()
		if err != nil {
			t.Fatalf("recv %d: %v", i, err)
		}
		if want := string(rune('a' + i)); got.GetRequestId() != want {
			t.Fatalf("frame %d: request_id = %q, want %q", i, got.GetRequestId(), want)
		}
	}
}

func TestConnRejectsOversizedFrame(t *testing.T) {
	a, b := pipe(t)
	cb := NewConn(b)

	go func() {
		var hdr []byte
		hdr = binary.AppendUvarint(hdr, MaxFrameBytes+1)
		_, _ = a.Write(hdr)
	}()
	_, err := cb.Recv()
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("err = %v, want ErrFrameTooLarge", err)
	}
}

func TestConnEOF(t *testing.T) {
	a, b := pipe(t)
	cb := NewConn(b)
	_ = a.Close()
	if _, err := cb.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}
