package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// guestPartsDir holds the scripts the CLI sends, exactly, for the guest
// NixOS test (nix/guest/tests/default.nix "laptop parity"), which runs
// them as dev in a real guest: sudo into /etc/repose/env, git's include
// precedence. This test fails when the scripts change and the files were
// not regenerated (go test ./internal/cli -run TestGuestPartsGolden -update).
const guestPartsDir = "testdata/guest-parts"

func TestGuestPartsGolden(t *testing.T) {
	gc := &gitCarry{
		Config: renderGitConfig([]gitEntry{
			{Key: "alias.st", Value: "status -sb", HasValue: true},
			{Key: "alias.lg", Value: "log --oneline", HasValue: true},
			{Key: "core.pager", Value: "delta", HasValue: true},
			{Key: "user.name", Value: "Lap Top", HasValue: true},
			{Key: "user.email", Value: "work@corp.example", HasValue: true},
		}),
		Checks: []gitCheck{{Key: "core.pager", Kind: "cmd", Value: "delta"}},
		HasID:  true,
	}
	files := map[string][]byte{
		"tz.sh":              []byte(tzPart("Asia/Tokyo")),
		"git.sh":             []byte(gitPartScript()),
		"git/config":         gc.Config,
		"git/check-000.key":  []byte("core.pager"),
		"git/check-000.kind": []byte("cmd"),
		"git/check-000.val":  []byte("delta"),
	}
	for rel, want := range files {
		p := filepath.Join(guestPartsDir, rel)
		if *updateGolden {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s is stale; run this test with -update (%v)", p, err)
		}
	}
}
