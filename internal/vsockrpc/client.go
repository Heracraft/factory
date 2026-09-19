package vsockrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
)

// ErrClosed is returned once a Client's connection has gone away.
var ErrClosed = errors.New("vsockrpc: connection closed")

// Client is hostd's side of the protocol: it multiplexes requests by
// request_id over one Conn and delivers unsolicited Notify messages to a
// callback. It is safe for concurrent use.
type Client struct {
	conn   *Conn
	notify func(*guestdv1.Notify)

	mu      sync.Mutex
	pending map[string]chan *guestdv1.Response
	closed  bool
	cause   error

	done chan struct{}
}

// NewClient starts the read loop over rwc. onNotify is called from that loop,
// so it must not block; hostd hands it a buffered channel.
func NewClient(rwc io.ReadWriteCloser, onNotify func(*guestdv1.Notify)) *Client {
	if onNotify == nil {
		onNotify = func(*guestdv1.Notify) {}
	}
	c := &Client{
		conn:    NewConn(rwc),
		notify:  onNotify,
		pending: make(map[string]chan *guestdv1.Response),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) readLoop() {
	var err error
	for {
		var env *guestdv1.Envelope
		env, err = c.conn.Recv()
		if err != nil {
			break
		}
		switch body := env.GetBody().(type) {
		case *guestdv1.Envelope_Notify:
			c.notify(body.Notify)
		case *guestdv1.Envelope_Response:
			c.mu.Lock()
			ch, ok := c.pending[env.GetRequestId()]
			if ok {
				delete(c.pending, env.GetRequestId())
			}
			c.mu.Unlock()
			if ok {
				ch <- body.Response
			}
			// A response with no waiter is a reply to a cancelled request;
			// dropping it is correct, and the deadline already reported.
		default:
			// A request from the guest is not part of the contract. Ignore it
			// rather than tear the connection down.
		}
	}
	c.shutdown(err)
}

func (c *Client) shutdown(cause error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	if cause == nil || errors.Is(cause, io.EOF) {
		cause = ErrClosed
	}
	c.cause = cause
	pending := c.pending
	c.pending = nil
	c.mu.Unlock()

	for _, ch := range pending {
		close(ch)
	}
	close(c.done)
	_ = c.conn.Close()
}

// Do sends one request and waits for its response. The caller's context bounds
// the wait; guestd applies its own deadline per handler.
func (c *Client) Do(ctx context.Context, req *guestdv1.Request) (*guestdv1.Response, error) {
	id := uuid.NewString()
	ch := make(chan *guestdv1.Response, 1)

	c.mu.Lock()
	if c.closed {
		err := c.cause
		c.mu.Unlock()
		return nil, err
	}
	c.pending[id] = ch
	c.mu.Unlock()

	env := &guestdv1.Envelope{RequestId: id, Body: &guestdv1.Envelope_Request{Request: req}}
	if err := c.conn.Send(env); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("send request: %w", err)
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			c.mu.Lock()
			err := c.cause
			c.mu.Unlock()
			return nil, err
		}
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// Close tears the connection down and fails every in-flight request.
func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	return nil
}

// Done is closed when the connection has gone away.
func (c *Client) Done() <-chan struct{} { return c.done }
