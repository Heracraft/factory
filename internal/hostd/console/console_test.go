package console

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotation(t *testing.T) {
	dir := t.TempDir()
	tl := New(filepath.Join(dir, "console.sock"), filepath.Join(dir, "console.log"))
	tl.Rotate = 100
	tl.Keep = 3
	chunk := bytes.Repeat([]byte("x"), 60)
	for i := 0; i < 8; i++ {
		if _, err := tl.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = tl.Close()
	for _, name := range []string{"console.log", "console.log.1", "console.log.2", "console.log.3"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s missing after rotation", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "console.log.4")); err == nil {
		t.Fatal("more than Keep old files")
	}
}

func TestRunCopiesSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "console.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	tl := New(sock, filepath.Join(dir, "console.log"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- tl.Run(ctx) }()
	c, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Write([]byte("[    0.000000] Linux version 6.12\n"))
	_ = c.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(dir, "console.log"))
		if bytes.Contains(b, []byte("Linux version")) {
			cancel()
			<-done
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatal("console line never written")
}
