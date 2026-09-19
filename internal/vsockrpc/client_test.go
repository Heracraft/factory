package vsockrpc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
)

// echoServer answers every request with a Ping result carrying the request id,
// after an optional delay, so the multiplexing can be checked by identity.
func echoServer(t *testing.T, conn *Conn, delay time.Duration) {
	t.Helper()
	go func() {
		for {
			env, err := conn.Recv()
			if err != nil {
				return
			}
			id := env.GetRequestId()
			go func() {
				time.Sleep(delay)
				_ = conn.Send(&guestdv1.Envelope{
					RequestId: id,
					Body: &guestdv1.Envelope_Response{Response: &guestdv1.Response{
						Ok:     true,
						Result: &guestdv1.Response_Ping{Ping: &guestdv1.PingResult{BootId: id}},
					}},
				})
			}()
		}
	}()
}

func TestClientMultiplexesByRequestID(t *testing.T) {
	a, b := pipe(t)
	echoServer(t, NewConn(b), 10*time.Millisecond)

	c := NewClient(a, nil)
	defer c.Close() //nolint:errcheck // test cleanup

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp, err := c.Do(ctx, &guestdv1.Request{Req: &guestdv1.Request_Ping{Ping: &guestdv1.Ping{}}})
			if err != nil {
				t.Errorf("do: %v", err)
				return
			}
			if !resp.GetOk() {
				t.Errorf("response not ok: %+v", resp.GetError())
			}
		}()
	}
	wg.Wait()
}

func TestClientDeliversNotifications(t *testing.T) {
	a, b := pipe(t)
	server := NewConn(b)

	got := make(chan *guestdv1.Notify, 1)
	c := NewClient(a, func(n *guestdv1.Notify) { got <- n })
	defer c.Close() //nolint:errcheck // test cleanup

	go func() {
		_ = server.Send(&guestdv1.Envelope{Body: &guestdv1.Envelope_Notify{Notify: &guestdv1.Notify{
			N: &guestdv1.Notify_Warning{Warning: &guestdv1.Warning{Kind: "disk_high", Detail: "93 percent"}},
		}}})
	}()

	select {
	case n := <-got:
		if n.GetWarning().GetKind() != "disk_high" {
			t.Fatalf("kind = %q, want disk_high", n.GetWarning().GetKind())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification arrived")
	}
}

func TestClientFailsPendingOnClose(t *testing.T) {
	a, b := pipe(t)
	// A server that reads and never answers.
	go func() {
		conn := NewConn(b)
		for {
			if _, err := conn.Recv(); err != nil {
				return
			}
		}
	}()

	c := NewClient(a, nil)
	errCh := make(chan error, 1)
	go func() {
		_, err := c.Do(context.Background(), &guestdv1.Request{Req: &guestdv1.Request_Ping{Ping: &guestdv1.Ping{}}})
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	_ = c.Close()

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("err = %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Do did not return after Close")
	}
}

func TestClientHonoursContextDeadline(t *testing.T) {
	a, b := pipe(t)
	go func() {
		conn := NewConn(b)
		for {
			if _, err := conn.Recv(); err != nil {
				return
			}
		}
	}()
	c := NewClient(a, nil)
	defer c.Close() //nolint:errcheck // test cleanup

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Do(ctx, &guestdv1.Request{Req: &guestdv1.Request_Ping{Ping: &guestdv1.Ping{}}}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
}
