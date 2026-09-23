package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

func TestParseCpSide(t *testing.T) {
	for arg, want := range map[string]cpSide{
		":logs/x.log":      {Remote: true, Path: "logs/x.log"},
		"izma:/tmp/t.json": {Remote: true, Project: "izma", Path: "/tmp/t.json"},
		"izma:":            {Remote: true, Project: "izma"},
		".":                {Path: "."},
		"./a:b":            {Path: "./a:b"},
		"/abs/a:b":         {Path: "/abs/a:b"},
		"dir/a:b":          {Path: "dir/a:b"},
		"x.log":            {Path: "x.log"},
	} {
		if got := parseCpSide(arg); got != want {
			t.Errorf("parseCpSide(%q) = %+v, want %+v", arg, got, want)
		}
	}
	for path, want := range map[string]string{"logs/x.log": "izma/logs/x.log", "": "izma", "/tmp/a": "/tmp/a", "~/.bashrc": "~/.bashrc"} {
		if got := (cpSide{Remote: true, Path: path}).guestPath("izma"); got != want {
			t.Errorf("guestPath(%q) = %q, want %q", path, got, want)
		}
	}
}

// I-201 both ways against the local sshd harness: `repose cp :logs/x.log
// .` from the checkout, and the reverse into the project by name.
func TestScpRemoteQuote(t *testing.T) {
	for in, want := range map[string]string{
		"proj/logs/x.log": "proj/logs/x.log",
		"proj/a b $HOME":  `proj/a\ b\ \$HOME`,
		"~/it's":          `~/it\'s`,
		"~":               "~",
		"/tmp/*.log":      "/tmp/*.log",
	} {
		if got := scpRemoteQuote(in); got != want {
			t.Errorf("scpRemoteQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCpBothWays(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	f := newRunFixture(t, fake)
	scpExtraArgs = []string{"-O"}
	t.Cleanup(func() { scpExtraArgs = nil })
	ctx := context.Background()
	if err := runRun(ctx, f.env, RunOptions{Name: testSlug, NoAttach: true}, false); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(f.guestRepo(), "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logs, "x.log"), []byte("line from the guest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := CpCmd(ctx, f.env, ":logs/x.log", out, false, ""); err != nil {
		t.Fatalf("cp from the guest: %v %s", err, f.env.ErrOut.(*discardWriter).buf.String())
	}
	if b, err := os.ReadFile(filepath.Join(out, "x.log")); err != nil || string(b) != "line from the guest\n" {
		t.Fatalf("copied = %q %v", b, err)
	}
	local := filepath.Join(out, "trace.json")
	if err := os.WriteFile(local, []byte(`{"from":"laptop"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CpCmd(ctx, f.env, local, testSlug+":tmp/trace.json", false, ""); err == nil {
		t.Fatal("copy into a directory that does not exist should fail like scp")
	}
	if err := CpCmd(ctx, f.env, local, testSlug+":logs/", false, ""); err != nil {
		t.Fatalf("cp to the guest: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(logs, "trace.json")); err != nil || string(b) != `{"from":"laptop"}` {
		t.Fatalf("in the guest = %q %v", b, err)
	}
	// The classic protocol (what -O and OpenSSH before 8.8 speak) hands the
	// remote path to the guest's shell: a space or a $ must survive it.
	odd := "a b $HOME.log"
	if err := os.WriteFile(filepath.Join(logs, odd), []byte("odd name\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CpCmd(ctx, f.env, ":logs/"+odd, out, false, ""); err != nil {
		t.Fatalf("cp of %q from the guest: %v", odd, err)
	}
	if b, err := os.ReadFile(filepath.Join(out, odd)); err != nil || string(b) != "odd name\n" {
		t.Fatalf("copied %q = %q %v", odd, b, err)
	}
	if err := CpCmd(ctx, f.env, "a", "b", false, ""); err == nil || err.(*exitError).code != ExitUsage {
		t.Errorf("two local sides: %v", err)
	}
}
