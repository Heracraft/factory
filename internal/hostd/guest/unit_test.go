package guest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unit properties are the H-2 sandbox (DECISIONS I-49); the golden is
// what host-conventions.md "Cloud Hypervisor invocation" lists.
func TestGuestUnitPropsGolden(t *testing.T) {
	dir := "/var/lib/repose/guests/0192f0a1-1111-7000-8000-000000000001"
	got := strings.Join(GuestUnitProps(Classes["large"], "hostd", "/var/lib/repose/guests", dir, "/dev/vg-guests/g-0192f0a1-1111-7000-8000-000000000001"), "\n") + "\n"
	want, err := os.ReadFile("testdata/unit.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("properties differ from testdata/unit.golden:\n%s", got)
	}
	for _, forbidden := range []string{"AF_INET", "AmbientCapabilities", "User=root"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("unit properties contain %q", forbidden)
		}
	}
}

// The guest directory layout lets the two unprivileged units bind their
// sockets while the hypervisor cannot touch hostd's own files.
func TestPrepareGuestDirLayout(t *testing.T) {
	h := newHarness(t, nil)
	dir := filepath.Join(h.cfg.GuestsDir, gid1)
	if err := h.m.prepareGuestDir(dir); err != nil {
		t.Fatal(err)
	}
	mode := func(p string) os.FileMode {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		return st.Mode().Perm() | (st.Mode() & os.ModeSticky)
	}
	if m := mode(h.cfg.GuestsDir); m != 0o711 {
		t.Fatalf("guests dir mode %o, want 711", m)
	}
	if m := mode(dir); m != os.FileMode(0o770)|os.ModeSticky {
		t.Fatalf("guest dir mode %v, want 1770", m)
	}
	if m := mode(filepath.Join(dir, "virtiofsd")); m != 0o750 {
		t.Fatalf("virtiofsd dir mode %o, want 750", m)
	}
	// Idempotent: a second boot of the same guest finds the layout in place.
	if err := h.m.prepareGuestDir(dir); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareGuestDirNamesTheMissingUser(t *testing.T) {
	h := newHarness(t, func(c *Config) {
		c.Lookup = func(name string) (int, int, error) { return 0, 0, os.ErrNotExist }
	})
	err := h.m.prepareGuestDir(filepath.Join(h.cfg.GuestsDir, gid1))
	if err == nil || !strings.Contains(err.Error(), "guest user hostd") {
		t.Fatalf("err %v", err)
	}
}
