package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// I-412: the carry writes the laptop's repose version to
// ~/.repose/cli-version, once per version.
func TestCarryCLIVersion(t *testing.T) {
	old := cliVersion
	t.Cleanup(func() { cliVersion = old })
	cliVersion = "v9.9.9"

	sent, err := addCarry(newGuestPayload(), carryOptions{})
	if err != nil || !slices.Contains(sent, "cli-version") {
		t.Fatalf("no cli-version part: %v %v", sent, err)
	}
	sent, err = addCarry(newGuestPayload(), carryOptions{Markers: map[string]string{"cli-version": carryHash([]byte("v9.9.9"))}})
	if err != nil || slices.Contains(sent, "cli-version") {
		t.Fatalf("sent again with the marker set: %v %v", sent, err)
	}

	home := t.TempDir()
	c := exec.Command("sh", "-ec", cliVersionPart("v9.9.9"))
	c.Env = append(os.Environ(), "HOME="+home)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	b, err := os.ReadFile(filepath.Join(home, ".repose", "cli-version"))
	if err != nil || string(b) != "v9.9.9\n" {
		t.Fatalf("cli-version: %q %v", b, err)
	}

	cliVersion = ""
	if sent, _ := addCarry(newGuestPayload(), carryOptions{}); slices.Contains(sent, "cli-version") {
		t.Fatal("sent with no version")
	}
}
