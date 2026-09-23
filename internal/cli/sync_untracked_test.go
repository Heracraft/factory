package cli

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The owner's teksafari.org checkout (2026-09-23): cms/node_modules was
// not in .gitignore and pnpm builds it out of symlinks to directories.
// The sync read one as a file ("is a directory") and sent nothing. Now
// dependency directories stay behind, named once, and a symlink outside
// them travels as a symlink.
func TestUntrackedSkipsDependencyDirsAndKeepsSymlinks(t *testing.T) {
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("cms/node_modules/.pnpm/@actions+exec@1.1.1/node_modules/@actions/exec/index.js", "x")
	mk("notes.md", "hello")
	mk("web/dist/app.js", "built")
	if err := os.Symlink(".pnpm/@actions+exec@1.1.1/node_modules/@actions/exec", filepath.Join(root, "cms/node_modules/exec")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("notes.md", filepath.Join(root, "latest.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	listed := []string{
		"cms/node_modules/.pnpm/@actions+exec@1.1.1/node_modules/@actions/exec/index.js",
		"cms/node_modules/exec", "notes.md", "latest.md", "emptydir", "web/dist/app.js",
	}
	kept, big, dirs, capped, err := filterUntracked(root, listed, []string{"dist"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(kept, ",") != "notes.md,latest.md" {
		t.Fatalf("kept %v, want notes.md and latest.md (dist excluded by sync.exclude, emptydir is a directory)", kept)
	}
	if len(big) != 0 || capped != 0 || strings.Join(dirs, ",") != "cms/node_modules" {
		t.Fatalf("big %v capped %d dirs %v, want only cms/node_modules skipped", big, capped, dirs)
	}
	b, err := tarFiles(root, kept)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(bytes.NewReader(b))
	got := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeSymlink {
			got[h.Name] = "->" + h.Linkname
		} else {
			c, _ := io.ReadAll(tr)
			got[h.Name] = string(c)
		}
	}
	if got["notes.md"] != "hello" || got["latest.md"] != "->notes.md" || len(got) != 2 {
		t.Fatalf("tar holds %v", got)
	}
	s := &SyncSummary{SkippedDirs: dirs}
	if w := strings.Join(s.Warnings(), "\n"); !strings.Contains(w, "Not sent: cms/node_modules") {
		t.Fatalf("warnings %q do not name cms/node_modules", w)
	}
}
