package cli

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

func TestStatusListsListeningProcesses(t *testing.T) {
	out := `LISTEN 0 511 0.0.0.0:5173 0.0.0.0:* users:(("node",pid=4242,fd=20))
LISTEN 0 511 [::]:5173 [::]:* users:(("node",pid=4242,fd=21))
LISTEN 0 4096 127.0.0.1:5432 0.0.0.0:*
LISTEN 0 128 0.0.0.0:22 0.0.0.0:*
LISTEN 0 5 127.0.0.1:6080 0.0.0.0:* users:(("systemd",pid=1,fd=40))
LISTEN 0 511 127.0.0.1:3000 0.0.0.0:* users:(("next-server (v1",pid=77,fd=3))
#ps
   4242  259200 419840 node
     77    1500  98304 next-server (v1
`
	var b bytes.Buffer
	writeListening(&b, parseStatusProcs(out))
	want := `  listening  next-server (v1 :3000 up 25m 96.0 MB
             node :5173 up 3d 410.0 MB
             :5432
`
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

// End to end against the fake api and the local sshd harness (whose
// "guest" is this machine): a listener this test opens shows in `repose
// status`, with this process's name, age and memory.
func TestStatusShowsTheGuestsListeners(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	ctx := context.Background()
	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	out := &discardWriter{}
	f.env.Out = out
	if err := StatusCmd(ctx, f.env, testSlug); err != nil {
		t.Fatal(err)
	}
	var line string
	for _, l := range strings.Split(out.buf.String(), "\n") {
		if strings.Contains(l, fmt.Sprintf(":%d up ", port)) {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("port %d not in status:\n%s", port, out.buf.String())
	}
	t.Logf("%s", line)
}
