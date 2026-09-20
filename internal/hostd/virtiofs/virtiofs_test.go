package virtiofs

import (
	"context"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/systemd"
)

func TestStartRendersUnprivilegedNamespaceSandbox(t *testing.T) {
	sd := systemd.NewFake()
	cfg := Config{SharedDir: "/run/repose/store-export", User: "virtiofsd", Group: "virtiofsd", SocketGroup: "hostd"}
	if err := Start(context.Background(), sd, cfg, "g1", "/var/lib/repose/guests/g1/virtiofsd/virtiofsd.sock"); err != nil {
		t.Fatal(err)
	}
	u := sd.Units["virtiofsd@g1"]
	if u == nil {
		t.Fatal("unit not started")
	}
	wantArgv := "virtiofsd --socket-path /var/lib/repose/guests/g1/virtiofsd/virtiofsd.sock --shared-dir /run/repose/store-export --sandbox namespace --cache auto --xattr --no-announce-submounts --socket-group hostd"
	if got := strings.Join(u.Argv, " "); got != wantArgv {
		t.Fatalf("argv %q", got)
	}
	wantProps := "User=virtiofsd Group=virtiofsd MemoryMax=1G Slice=guests.slice"
	if got := strings.Join(u.Props, " "); got != wantProps {
		t.Fatalf("props %q", got)
	}
}
