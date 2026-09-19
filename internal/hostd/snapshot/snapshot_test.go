package snapshot

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/lvm"
	"github.com/heracraft/repose/internal/hostd/shell"
)

func TestFileBlobRoundTrip(t *testing.T) {
	b := &FileBlob{Dir: t.TempDir()}
	ctx := context.Background()
	n, err := b.Upload(ctx, "u1/p1/2026-09-19T03:00:00Z.img.zst", strings.NewReader("hello"), map[string]string{"guest_id": "g"})
	if err != nil || n != 5 {
		t.Fatalf("upload %d %v", n, err)
	}
	var out bytes.Buffer
	if err := b.Download(ctx, "u1/p1/2026-09-19T03:00:00Z.img.zst", &out); err != nil || out.String() != "hello" {
		t.Fatalf("download %q %v", out.String(), err)
	}
	if ok, _ := b.Exists(ctx, "u1/p1/nope"); ok {
		t.Fatal("missing blob reported present")
	}
	if _, err := b.Upload(ctx, "../escape", strings.NewReader("x"), nil); err == nil {
		t.Fatal("path traversal accepted")
	}
}

func TestFakeStreamerRoundTrip(t *testing.T) {
	l := lvm.NewFake()
	ctx := context.Background()
	_ = l.CreateVolume(ctx, "g-1", 10)
	l.SetData("g-1", []byte("data"))
	s := &FakeStreamer{LVM: l}
	r, err := s.Read(ctx, "/dev/vg-guests/g-1")
	if err != nil {
		t.Fatal(err)
	}
	_ = l.CreateVolume(ctx, "g-2", 10)
	if err := s.Write(ctx, "/dev/vg-guests/g-2", r); err != nil {
		t.Fatal(err)
	}
	if string(l.GetData("g-2")) != "data" {
		t.Fatal("bytes did not round-trip")
	}
}

func TestPipelineWithRealTools(t *testing.T) {
	for _, tool := range []string{"dd", "zstd"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	src, dst := t.TempDir()+"/src", t.TempDir()+"/dst"
	payload := bytes.Repeat([]byte("repose"), 100000)
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{R: shell.Exec{}}
	ctx := context.Background()
	r, err := p.Read(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	if _, err := compressed.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if compressed.Len() >= len(payload)/10 {
		t.Fatalf("zstd did not compress: %d bytes", compressed.Len())
	}
	if err := p.Write(ctx, dst, &compressed); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !bytes.Equal(got, payload) {
		t.Fatal("restored bytes differ")
	}
}
