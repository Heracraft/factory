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

// The login share (I-278) is the shape checked on host-01 on 2026-09-26:
// unprivileged, namespace sandbox, no page cache, dev mapped to the
// serving account, and no --xattr (translation refuses POSIX ACLs).
func TestStartAuthRendersTranslatedUncachedShare(t *testing.T) {
	sd := systemd.NewFake()
	cfg := AuthConfig{SharedDir: "/var/lib/repose/users/u1/claude-auth", User: "repose-auth", Group: "repose-auth", UID: 991, GID: 990, SocketGroup: "hostd"}
	if err := StartAuth(context.Background(), sd, cfg, "g1", "/var/lib/repose/guests/g1/virtiofsd-auth/virtiofsd.sock"); err != nil {
		t.Fatal(err)
	}
	u := sd.Units["virtiofsd-auth@g1"]
	if u == nil {
		t.Fatal("unit not started")
	}
	wantArgv := "virtiofsd --socket-path /var/lib/repose/guests/g1/virtiofsd-auth/virtiofsd.sock --shared-dir /var/lib/repose/users/u1/claude-auth --sandbox namespace --cache never --translate-uid map:1000:991:1 --translate-gid map:1000:990:1 --socket-group hostd"
	if got := strings.Join(u.Argv, " "); got != wantArgv {
		t.Fatalf("argv %q", got)
	}
	wantProps := "User=repose-auth Group=repose-auth MemoryMax=64M Slice=guests.slice"
	if got := strings.Join(u.Props, " "); got != wantProps {
		t.Fatalf("props %q", got)
	}
	if err := StopAuth(context.Background(), sd, "g1"); err != nil || sd.Units["virtiofsd-auth@g1"].Active {
		t.Fatalf("stop: %v", err)
	}
}
