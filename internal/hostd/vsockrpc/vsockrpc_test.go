package vsockrpc

import (
	"context"
	"net"
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
)

func TestCallAndNotify(t *testing.T) {
	a, b := net.Pipe()
	srv := Serve(context.Background(), b, HandlerFunc(func(_ context.Context, req *guestdv1.Request) *guestdv1.Response {
		switch req.Req.(type) {
		case *guestdv1.Request_Ping:
			return &guestdv1.Response{Ok: true, Result: &guestdv1.Response_Ping{Ping: &guestdv1.PingResult{Version: "t", BootId: "b1"}}}
		default:
			return &guestdv1.Response{Ok: false, Error: &guestdv1.Error{Code: "invalid_argument", Message: "nope"}}
		}
	}))
	cl := NewClient(a, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := cl.Call(ctx, &guestdv1.Request{Req: &guestdv1.Request_Ping{Ping: &guestdv1.Ping{}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetPing().GetBootId() != "b1" {
		t.Fatalf("bad ping result: %v", resp)
	}
	_, err = cl.Call(ctx, &guestdv1.Request{Req: &guestdv1.Request_Thaw{Thaw: &guestdv1.Thaw{}}})
	re, ok := err.(*RemoteError)
	if !ok || re.Code != "invalid_argument" {
		t.Fatalf("expected remote error, got %v", err)
	}
	if err := srv.Notify(&guestdv1.Notify{N: &guestdv1.Notify_Ready{Ready: &guestdv1.Ready{BootId: "b1"}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-cl.Notifications():
		if n.GetReady().GetBootId() != "b1" {
			t.Fatalf("bad notify %v", n)
		}
	case <-ctx.Done():
		t.Fatal("no notification")
	}
	_ = srv.Close()
	select {
	case <-cl.Done():
	case <-ctx.Done():
		t.Fatal("client did not observe close")
	}
	if _, err := cl.Call(ctx, &guestdv1.Request{Req: &guestdv1.Request_Ping{Ping: &guestdv1.Ping{}}}); err == nil {
		t.Fatal("call on closed client should fail")
	}
}
