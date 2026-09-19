package snapshot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/heracraft/repose/internal/hostd/lvm"
	"github.com/heracraft/repose/internal/hostd/shell"
)

// Streamer moves bytes between a block device and a stream.
type Streamer interface {
	// Read returns the compressed contents of dev.
	Read(ctx context.Context, dev string) (io.ReadCloser, error)
	// Write decompresses r onto dev.
	Write(ctx context.Context, dev string, r io.Reader) error
}

// Pipeline is the real Streamer: dd if=dev bs=4M | zstd -T4 -3 on the way
// out, zstd -d | dd of=dev bs=4M conv=sparse on the way in.
type Pipeline struct {
	R shell.Runner
}

type procReader struct {
	io.ReadCloser
	wait []func() error
}

func (p *procReader) Close() error {
	err := p.ReadCloser.Close()
	for _, w := range p.wait {
		if e := w(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// Read implements Streamer.
func (p *Pipeline) Read(ctx context.Context, dev string) (io.ReadCloser, error) {
	dd, err := p.R.Start(ctx, nil, "dd", "if="+dev, "bs=4M", "status=none")
	if err != nil {
		return nil, fmt.Errorf("dd: %w", err)
	}
	go func() { _, _ = io.Copy(io.Discard, dd.Stderr()) }() // status=none keeps it empty; drained so dd never blocks
	zs, err := p.R.Start(ctx, dd.Stdout(), "zstd", "-T4", "-3", "-q", "-c")
	if err != nil {
		_ = dd.Kill() // zstd failed to start; nothing consumed dd's output
		return nil, fmt.Errorf("zstd: %w", err)
	}
	go func() { _, _ = io.Copy(io.Discard, zs.Stderr()) }() // -q keeps it empty; drained so zstd never blocks
	return &procReader{ReadCloser: zs.Stdout(), wait: []func() error{zs.Wait, dd.Wait}}, nil
}

// Write implements Streamer.
func (p *Pipeline) Write(ctx context.Context, dev string, r io.Reader) error {
	zs, err := p.R.Start(ctx, r, "zstd", "-d", "-q", "-c")
	if err != nil {
		return fmt.Errorf("zstd -d: %w", err)
	}
	go func() { _, _ = io.Copy(io.Discard, zs.Stderr()) }() // drained so zstd never blocks
	dd, err := p.R.Start(ctx, zs.Stdout(), "dd", "of="+dev, "bs=4M", "conv=sparse,fsync", "status=none")
	if err != nil {
		_ = zs.Kill() // dd failed to start
		return fmt.Errorf("dd: %w", err)
	}
	go func() { _, _ = io.Copy(io.Discard, dd.Stdout()) }() // dd writes nothing to stdout; drained anyway
	go func() { _, _ = io.Copy(io.Discard, dd.Stderr()) }() // status=none keeps it empty
	ddErr := dd.Wait()
	zsErr := zs.Wait()
	if zsErr != nil {
		return fmt.Errorf("zstd -d: %w", zsErr)
	}
	if ddErr != nil {
		return fmt.Errorf("dd: %w", ddErr)
	}
	return nil
}

// FakeStreamer copies bytes between the fake LVM's volumes and streams.
type FakeStreamer struct {
	LVM  *lvm.Fake
	Fail error
}

func devName(dev string) string {
	for i := len(dev) - 1; i >= 0; i-- {
		if dev[i] == '/' {
			return dev[i+1:]
		}
	}
	return dev
}

// Read implements Streamer.
func (f *FakeStreamer) Read(_ context.Context, dev string) (io.ReadCloser, error) {
	if f.Fail != nil {
		return nil, f.Fail
	}
	data := f.LVM.GetData(devName(dev))
	if data == nil {
		return nil, errors.New("fake streamer: no such volume " + dev)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Write implements Streamer.
func (f *FakeStreamer) Write(_ context.Context, dev string, r io.Reader) error {
	if f.Fail != nil {
		return f.Fail
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.LVM.SetData(devName(dev), b)
	return nil
}
