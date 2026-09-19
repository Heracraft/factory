// Package vsockrpc carries the hostd ⇄ guestd protocol of
// docs/interfaces/vsock-guestd.md: length-prefixed protobuf Envelopes over a
// vsock connection, or over a unix socket in dev mode. It holds both sides of
// the wire so hostd's client and guestd's server cannot drift apart.
package vsockrpc

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"google.golang.org/protobuf/proto"
)

// MaxFrameBytes bounds a single frame. Exec and Switch results are capped at
// 64 KB and 32 KB by the contract, so a megabyte is generous; the cap exists so
// a corrupt length cannot make guestd allocate the guest's memory.
const MaxFrameBytes = 1 << 20

// ErrFrameTooLarge is returned when a peer announces a frame over MaxFrameBytes.
var ErrFrameTooLarge = errors.New("vsockrpc: frame exceeds maximum size")

// Conn is one framed connection. Reads are single-threaded by contract (one
// reader goroutine); writes are serialised by a mutex so handlers running
// concurrently can respond without interleaving.
type Conn struct {
	rwc io.ReadWriteCloser
	br  *bufio.Reader

	wmu  sync.Mutex
	wbuf []byte
}

// NewConn wraps a stream in the framing.
func NewConn(rwc io.ReadWriteCloser) *Conn {
	return &Conn{rwc: rwc, br: bufio.NewReaderSize(rwc, 16<<10)}
}

// Recv reads one Envelope. It returns io.EOF when the peer closed cleanly.
func (c *Conn) Recv() (*guestdv1.Envelope, error) {
	n, err := binary.ReadUvarint(c.br)
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	if n > MaxFrameBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.br, buf); err != nil {
		return nil, fmt.Errorf("vsockrpc: short frame: %w", err)
	}
	env := &guestdv1.Envelope{}
	if err := proto.Unmarshal(buf, env); err != nil {
		return nil, fmt.Errorf("vsockrpc: decode envelope: %w", err)
	}
	return env, nil
}

// Send writes one Envelope.
func (c *Conn) Send(env *guestdv1.Envelope) error {
	body, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("vsockrpc: encode envelope: %w", err)
	}
	if len(body) > MaxFrameBytes {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(body))
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.wbuf = c.wbuf[:0]
	c.wbuf = binary.AppendUvarint(c.wbuf, uint64(len(body)))
	c.wbuf = append(c.wbuf, body...)
	if _, err := c.rwc.Write(c.wbuf); err != nil {
		return fmt.Errorf("vsockrpc: write frame: %w", err)
	}
	return nil
}

// Close closes the underlying stream. It is safe to call more than once only
// insofar as the underlying stream allows it; callers use it once.
func (c *Conn) Close() error { return c.rwc.Close() }
