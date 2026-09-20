package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The api refuses anything but an IANA zone name (DECISIONS I-104); the CLI
// must send one or nothing.
func TestLocalTZFromEnv(t *testing.T) {
	t.Setenv("TZ", "Europe/Paris")
	if got := localTZ(); got != "Europe/Paris" {
		t.Fatalf("localTZ with TZ=Europe/Paris: %q", got)
	}
	t.Setenv("TZ", "EAT") // an abbreviation, not a zone
	if got := localTZ(); got == "EAT" {
		t.Fatalf("localTZ passed an abbreviation through: %q", got)
	}
}

func TestTZFromLocaltimeSymlink(t *testing.T) {
	dir := t.TempDir()
	zi := filepath.Join(dir, "usr", "share", "zoneinfo", "Africa")
	if err := os.MkdirAll(zi, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zi, "Nairobi"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "localtime")
	if err := os.Symlink(filepath.Join(zi, "Nairobi"), link); err != nil {
		t.Fatal(err)
	}
	if got := tzFromLocaltime(link); got != "Africa/Nairobi" {
		t.Fatalf("tzFromLocaltime: %q", got)
	}
	if got := tzFromLocaltime(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing link: %q", got)
	}
}
