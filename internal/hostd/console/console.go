// Package console captures a guest's serial output. Cloud Hypervisor cannot
// reopen a log file for rotation, so the runner passes `--serial socket=`
// and a Tailer copies the socket into console.log, rotating at 64 MB and
// keeping 3 old files. Fluent Bit tails the current file.
package console

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Defaults from workstream 03 §5.13.
const (
	RotateBytes = 64 << 20
	Keep        = 3
)

// Tailer copies one guest's console socket into its log.
type Tailer struct {
	Socket string
	Log    string
	Rotate int64
	Keep   int
	// Retry is the reconnect interval while the socket is absent.
	Retry time.Duration

	mu      sync.Mutex
	written int64
	f       *os.File
}

// New returns a Tailer with the documented defaults.
func New(socket, log string) *Tailer {
	return &Tailer{Socket: socket, Log: log, Rotate: RotateBytes, Keep: Keep, Retry: 500 * time.Millisecond}
}

func (t *Tailer) open() error {
	f, err := os.OpenFile(t.Log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close() // stat failed; nothing was written
		return err
	}
	t.f, t.written = f, st.Size()
	return nil
}

func (t *Tailer) rotate() error {
	if t.f != nil {
		_ = t.f.Close() // rotating; the old file is renamed next
		t.f = nil
	}
	for i := t.Keep; i >= 1; i-- {
		from := t.Log
		if i > 1 {
			from = fmt.Sprintf("%s.%d", t.Log, i-1)
		}
		to := fmt.Sprintf("%s.%d", t.Log, i)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return t.open()
}

// Write appends to the log, rotating when the size cap is reached.
func (t *Tailer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		if err := t.open(); err != nil {
			return 0, err
		}
	}
	if t.written+int64(len(p)) > t.Rotate {
		if err := t.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := t.f.Write(p)
	t.written += int64(n)
	return n, err
}

// Close closes the log.
func (t *Tailer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.f == nil {
		return nil
	}
	err := t.f.Close()
	t.f = nil
	return err
}

// Run connects to the socket (retrying while the hypervisor is starting)
// and copies until ctx ends. A dropped connection is retried, since a
// guest restart recreates the socket.
func (t *Tailer) Run(ctx context.Context) error {
	defer func() { _ = t.Close() }() // the log is append-only; nothing is lost on a close error
	if err := os.MkdirAll(filepath.Dir(t.Log), 0o750); err != nil {
		return err
	}
	for {
		var d net.Dialer
		c, err := d.DialContext(ctx, "unix", t.Socket)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(t.Retry):
				continue
			}
		}
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = c.Close() // unblocks the read loop; ctx.Err is returned below
			case <-done:
			}
		}()
		buf := make([]byte, 32<<10)
		for {
			n, rerr := c.Read(buf)
			if n > 0 {
				if _, werr := t.Write(buf[:n]); werr != nil {
					close(done)
					_ = c.Close() // giving up on this connection; the write error is returned
					return werr
				}
			}
			if rerr != nil {
				break
			}
		}
		close(done)
		_ = c.Close() // the read loop ended; the socket is reconnected or ctx is done
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(t.Retry):
		}
	}
}
