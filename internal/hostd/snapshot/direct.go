package snapshot

import (
	"fmt"
	"os"
	"sync"
	"unsafe"
)

// A restore writes around the page cache. Written through it, the 4.3 GB
// of a 5 GB volume went dirty in a second and then drained into the new
// thin volume at 34 MB/s: 126 s of fsync on host-01, whose data disk takes
// 600 MB/s. With O_DIRECT and several writes in flight the same stream
// took 5 to 8 s (DECISIONS I-403).
const (
	// restoreWriters is how many records are written at once: on host-01
	// one took 8.0 s, four 5.6 s, sixteen 5.0 s, sixty-four 6.7 s.
	restoreWriters = 8
	// directAlign is what O_DIRECT needs of an offset, a length and a
	// buffer; 4096 covers both 512- and 4096-byte logical blocks.
	directAlign = 4096
)

// extentWriter writes records to a device. In direct mode, restoreWriters
// goroutines take records from jobs; buffers come from a fixed pool, which
// bounds the memory a restore holds to restoreWriters+2 records. In
// buffered mode (no O_DIRECT handle, or a record that is not aligned) each
// record is written in the caller.
type extentWriter struct {
	dev    *os.File
	direct *os.File // nil once buffered
	free   chan []byte
	jobs   chan extentJob
	done   sync.WaitGroup // the goroutines
	flight sync.WaitGroup // records handed to them and not yet written
	end    uint64         // the highest byte any handed-out record reaches
	mu     sync.Mutex
	err    error
	closed bool
}

type extentJob struct {
	off    uint64
	buf    []byte // the pool buffer, released after the write
	lo, hi int    // what is written: buf[lo:hi] at off
}

func newExtentWriter(dev, direct *os.File) *extentWriter {
	w := &extentWriter{dev: dev, direct: direct}
	n := 1
	if direct != nil {
		n = restoreWriters + 2
	}
	w.free = make(chan []byte, n)
	for range n {
		w.free <- alignedBuffer(extentChunk)
	}
	if direct != nil {
		w.jobs = make(chan extentJob, restoreWriters)
		for range restoreWriters {
			w.done.Add(1)
			go w.run(direct)
		}
	}
	return w
}

// alignedBuffer is n bytes starting on a directAlign boundary.
func alignedBuffer(n int) []byte {
	b := make([]byte, n+directAlign)
	skip := 0
	if r := int(uintptr(unsafe.Pointer(&b[0])) % directAlign); r != 0 {
		skip = directAlign - r
	}
	return b[skip : skip+n : skip+n]
}

func (w *extentWriter) run(direct *os.File) {
	defer w.done.Done()
	for j := range w.jobs {
		if w.failed() == nil {
			if _, err := direct.WriteAt(j.buf[j.lo:j.hi], int64(j.off)); err != nil {
				w.fail(fmt.Errorf("write at %d: %w", j.off, err))
			}
		}
		w.release(j.buf)
		w.flight.Done()
	}
}

func (w *extentWriter) fail(err error) {
	w.mu.Lock()
	if w.err == nil {
		w.err = err
	}
	w.mu.Unlock()
}

func (w *extentWriter) failed() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.err
}

// buffer waits for a free record buffer.
func (w *extentWriter) buffer() ([]byte, error) {
	if err := w.failed(); err != nil {
		return nil, err
	}
	return <-w.free, nil
}

func (w *extentWriter) release(buf []byte) { w.free <- buf }

// write hands buf (from buffer) over to be released once buf[lo:hi] is
// written at off. A record that overlaps one still in flight waits for
// those to land, so the device ends up as a one-at-a-time restore would
// leave it; the stream's records come in order and never overlap, so in
// practice that does not happen.
func (w *extentWriter) write(off uint64, buf []byte, lo, hi int) error {
	n := uint64(hi - lo)
	if w.direct != nil && (off%directAlign != 0 || n%directAlign != 0 || lo%directAlign != 0) {
		// The page cache would read the record's first or last page from
		// the device to fill it in, possibly before a direct write to the
		// rest of that page lands; drain first and stay buffered.
		w.flight.Wait()
		w.direct = nil
	}
	if w.direct == nil {
		_, err := w.dev.WriteAt(buf[lo:hi], int64(off))
		w.release(buf)
		if err != nil {
			w.fail(fmt.Errorf("write at %d: %w", off, err))
		}
		return w.failed()
	}
	if off < w.end {
		w.flight.Wait()
	}
	w.end = max(w.end, off+n)
	w.flight.Add(1)
	w.jobs <- extentJob{off: off, buf: buf, lo: lo, hi: hi}
	return w.failed()
}

// close waits for every record handed out and returns the first error.
// It is safe to call twice.
func (w *extentWriter) close() error {
	if !w.closed {
		w.closed = true
		if w.jobs != nil {
			close(w.jobs)
			w.done.Wait()
		}
	}
	return w.failed()
}
