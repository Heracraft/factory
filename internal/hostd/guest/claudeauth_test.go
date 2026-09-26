package guest

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// A guest with a user id boots with that user's login share (I-278): the
// directory exists 0700, its own virtiofsd serves it translated and
// uncached, and the hypervisor gets it as a second --fs.
func TestCreateAttachesTheUsersLoginShare(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	share := filepath.Join(h.cfg.UsersDir, "user-1", "claude-auth")
	fi, err := os.Stat(share)
	if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf("share %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.UsersDir, "user-1", authLastGuest)); err != nil {
		t.Fatalf("marker: %v", err)
	}
	dir := filepath.Join(h.cfg.GuestsDir, gid1)
	v := h.sd.Units["virtiofsd-auth@"+gid1]
	if v == nil || !v.Active {
		t.Fatalf("auth virtiofsd %+v", v)
	}
	argv := strings.Join(v.Argv, " ")
	for _, want := range []string{"--shared-dir " + share + " ", "--cache never", "--translate-uid map:1000:", "--sandbox namespace"} {
		if !strings.Contains(argv, want) {
			t.Fatalf("auth virtiofsd argv lacks %q: %s", want, argv)
		}
	}
	if !slices.Contains(v.Props, "User=repose-auth") {
		t.Fatalf("auth virtiofsd props %v", v.Props)
	}
	chArgv := strings.Join(h.sd.Units["guest@"+gid1].Argv, " ")
	if !strings.Contains(chArgv, "--fs tag=claude-auth,socket="+filepath.Join(dir, "virtiofsd-auth", "virtiofsd.sock")) {
		t.Fatalf("hypervisor argv lacks the login share: %s", chArgv)
	}
	written, _ := os.ReadFile(filepath.Join(dir, "ch.args"))
	if !strings.Contains(string(written), "tag=claude-auth") {
		t.Fatal("ch.args does not record the login share")
	}

	// Stop tears the share's virtiofsd down with the guest's; the
	// directory, and the login in it, stay.
	h.mustOK(cmd(&hostdv1.StopGuest{GuestId: gid1}))
	if h.sd.Units["virtiofsd-auth@"+gid1].Active {
		t.Fatal("auth virtiofsd survived stop")
	}
	if _, err := os.Stat(share); err != nil {
		t.Fatalf("share removed on stop: %v", err)
	}
}

// Two guests of one user share one directory; nothing per guest is in it.
func TestTwoGuestsOfOneUserShareOneDirectory(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	h.create(gid2)
	a := h.sd.Units["virtiofsd-auth@"+gid1].Argv
	b := h.sd.Units["virtiofsd-auth@"+gid2].Argv
	if a[slices.Index(a, "--shared-dir")+1] != b[slices.Index(b, "--shared-dir")+1] {
		t.Fatalf("different shares: %v / %v", a, b)
	}
}

// A guest with no usable user id boots without a share rather than with a
// path built from whatever the id was.
func TestNoUserIDNoLoginShare(t *testing.T) {
	for _, id := range []string{"", "../etc", "a/b"} {
		t.Run(id, func(t *testing.T) {
			h := newHarness(t, nil)
			req := createReq(gid1)
			req.SystemClosure = h.closure
			req.UserId = id
			h.mustOK(cmd(req))
			if _, ok := h.sd.Units["virtiofsd-auth@"+gid1]; ok {
				t.Fatal("auth virtiofsd started without a user id")
			}
			if strings.Contains(strings.Join(h.sd.Units["guest@"+gid1].Argv, " "), "claude-auth") {
				t.Fatal("hypervisor got a login share without a user id")
			}
			if entries, _ := os.ReadDir(h.cfg.UsersDir); len(entries) != 0 {
				t.Fatalf("users dir has %v", entries)
			}
		})
	}
}

// A share that cannot be started does not stop the guest: it boots
// without it and the user signs in there as before.
func TestLoginShareFailureStillBoots(t *testing.T) {
	h := newHarness(t, func(c *Config) {
		c.Lookup = func(name string) (int, int, error) {
			if name == "repose-auth" {
				return 0, 0, os.ErrNotExist
			}
			return os.Getuid(), os.Getgid(), nil
		}
	})
	h.create(gid1)
	if h.guest(gid1).State != StateRunning {
		t.Fatalf("state %s", h.guest(gid1).State)
	}
	if strings.Contains(strings.Join(h.sd.Units["guest@"+gid1].Argv, " "), "claude-auth") {
		t.Fatal("hypervisor got a share that never started")
	}
}

// The sweep keeps a share while its user has a guest here, and removes it
// AuthKeep after the last one went. A share with no marker starts its clock.
func TestSweepAuthShares(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1) // user-1 has a guest
	old := time.Now().Add(-31 * 24 * time.Hour)
	mk := func(user string, marker bool) string {
		d := filepath.Join(h.cfg.UsersDir, user)
		if err := os.MkdirAll(filepath.Join(d, "claude-auth"), 0o700); err != nil {
			t.Fatal(err)
		}
		if marker {
			if err := touch(filepath.Join(d, authLastGuest), old); err != nil {
				t.Fatal(err)
			}
		}
		return d
	}
	if err := os.Chtimes(filepath.Join(h.cfg.UsersDir, "user-1", authLastGuest), old, old); err != nil {
		t.Fatal(err)
	}
	gone := mk("user-gone", true)
	fresh := mk("user-nomarker", false)

	h.m.SweepAuthShares(context.Background())

	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("share of a user gone 31 days survived: %v", err)
	}
	for _, u := range []string{"user-1", "user-nomarker"} {
		fi, err := os.Stat(filepath.Join(h.cfg.UsersDir, u, authLastGuest))
		if err != nil || time.Since(fi.ModTime()) > time.Minute {
			t.Fatalf("%s marker not stamped: %v %v", u, fi, err)
		}
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal(err)
	}
}

// The host switch (repose.host.claudeLoginShare, hostd
// --claude-login-share=false) boots guests as before I-278.
func TestNoAuthShareSwitch(t *testing.T) {
	h := newHarness(t, func(c *Config) { c.NoAuthShare = true })
	h.create(gid1)
	if _, ok := h.sd.Units["virtiofsd-auth@"+gid1]; ok {
		t.Fatal("auth virtiofsd started with the share switched off")
	}
	if strings.Contains(strings.Join(h.sd.Units["guest@"+gid1].Argv, " "), "claude-auth") {
		t.Fatal("hypervisor got a login share with the share switched off")
	}
}
