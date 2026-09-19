package nixbuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/gcroot"
	"github.com/heracraft/repose/internal/hostd/shell"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMapEvalErrorFixtures(t *testing.T) {
	cases := []struct {
		file, wantFirst string
		wantLine        int32
	}{
		{"syntax.stderr", "syntax error, unexpected ';' at fragment.nix:1:49", 1},
		{"missing.stderr", "attribute 'ripgrepp' missing at fragment.nix:1:36 (did you mean ripgrep?)", 1},
		{"fetch.stderr", "eval-time fetch not allowed at fragment.nix:1:18; use pkgs.fetchurl { url = ...; hash = ...; }", 1},
		{"abspath.stderr", "access to absolute path '/etc/passwd' is forbidden in pure evaluation mode (use '--impure' to override) at fragment.nix:1:18; a fragment may only read files it carries", 1},
	}
	for _, c := range cases {
		e := MapEvalError(fixture(t, c.file))
		if e.Code != "eval_failed" {
			t.Errorf("%s: code %s", c.file, e.Code)
		}
		if got := firstLine(e.Message); got != c.wantFirst {
			t.Errorf("%s: first line %q, want %q", c.file, got, c.wantFirst)
		}
		if e.FragmentLine != c.wantLine {
			t.Errorf("%s: fragment_line %d, want %d", c.file, e.FragmentLine, c.wantLine)
		}
		if !strings.Contains(e.Message, "\n\nerror:") {
			t.Errorf("%s: verbatim output missing after the summary", c.file)
		}
	}
}

func TestMapBuildErrorAndTimeouts(t *testing.T) {
	e := MapBuildError(fixture(t, "build.stderr"))
	if e.Code != "build_failed" || firstLine(e.Message) != "build of fails-1.0 failed" {
		t.Fatalf("build: %s %q", e.Code, firstLine(e.Message))
	}
	if !strings.Contains(e.Message, "> boom") {
		t.Fatal("verbatim builder log missing")
	}
	bt := BuildTimeout(1800, fixture(t, "build.stderr"))
	if bt.Code != "build_timeout" || bt.Message != "build exceeded 1800 s; last derivation: fails-1.0" {
		t.Fatalf("timeout: %s %q", bt.Code, bt.Message)
	}
	et := EvalTimeout(60)
	if et.Code != "eval_failed" || et.Message != "evaluation exceeded 60 s" {
		t.Fatalf("eval timeout: %v", et)
	}
}

func fakeClosure(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	c := filepath.Join(dir, "sys")
	_ = os.MkdirAll(c, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "bzImage"), []byte("k"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "initrd"), []byte("i"), 0o644)
	_ = os.Symlink(filepath.Join(dir, "bzImage"), filepath.Join(c, "kernel"))
	_ = os.Symlink(filepath.Join(dir, "initrd"), filepath.Join(c, "initrd"))
	_ = os.WriteFile(filepath.Join(c, "init"), []byte("#!"), 0o755)
	_ = os.WriteFile(filepath.Join(c, "kernel-params"), []byte("loglevel=4 reboot=t\n"), 0o644)
	return c
}

func TestRealBuildFlow(t *testing.T) {
	closure := fakeClosure(t)
	base := t.TempDir()
	_ = os.MkdirAll(filepath.Join(base, "abc123", "nix"), 0o755)
	_ = os.WriteFile(filepath.Join(base, "abc123", "nix", "flake.nix"), []byte("{}"), 0o644)
	r := &shell.Fake{Scripts: []shell.Script{
		{Prefix: []string{"timeout", "60", "nix", "eval"}, Result: shell.Result{Stdout: []byte("/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-guest.drv\n")}},
		{Prefix: []string{"timeout", "1800", "nix", "build"}, Result: shell.Result{Stdout: []byte(closure + "\n"), Stderr: []byte("building '/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-guest.drv'...\ncopying path\n")}},
		{Prefix: []string{"nix", "path-info", "-S"}, Result: shell.Result{Stdout: []byte(closure + "\t5368709120\n")}},
	}}
	b := (&Real{R: r, BuildsDir: t.TempDir(), BaseDir: base, Roots: gcroot.Roots{Dir: t.TempDir()}, Timeout: "timeout"}).Defaults()
	var lines []string
	res, err := b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r1", Fragment: []byte("{}"), BaseRef: "abc123",
		Limits: Limits{EvalS: 60, BuildS: 1800, Cores: 8, ClosureBytes: 20 << 30}}, func(l string) { lines = append(lines, l) })
	if err != nil {
		t.Fatal(err)
	}
	if res.SystemClosure != closure || res.ClosureBytes != 5368709120 || !strings.HasSuffix(res.Kernel, "bzImage") {
		t.Fatalf("result %+v", res)
	}
	evalCall := strings.Join(r.CallsWithPrefix("timeout", "60", "nix", "eval")[0], " ")
	if !strings.Contains(evalCall, "--override-input fragment path:"+filepath.Join(b.BuildsDir, "r1")) || !strings.Contains(evalCall, "--option restrict-eval true") || !strings.Contains(evalCall, "#guestSystem.config.system.build.toplevel.drvPath") {
		t.Fatalf("eval argv: %s", evalCall)
	}
	buildCall := strings.Join(r.CallsWithPrefix("timeout", "1800", "nix", "build")[0], " ")
	if !strings.Contains(buildCall, "--cores 8") || !strings.Contains(buildCall, "--option sandbox true") || !strings.HasSuffix(buildCall, "nixos-system-guest.drv^*") {
		t.Fatalf("build argv: %s", buildCall)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "copying path") {
		t.Fatalf("log lines not streamed: %v", lines)
	}
	if got, _ := b.Roots.Get("rev-p1-r1"); got != closure {
		t.Fatalf("gc root not set: %q", got)
	}
	if _, err := os.Stat(filepath.Join(b.BuildsDir, "r1", "fragment.nix")); err != nil {
		t.Fatal("fragment not written")
	}

	// Closure cap.
	r.Scripts = append([]shell.Script{
		{Prefix: []string{"nix", "path-info", "-rS"}, Result: shell.Result{Stdout: []byte("/nix/store/x-cuda\t30000000000\n/nix/store/y-glibc\t30000000\n")}},
	}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r2", BaseRef: "abc123", Limits: Limits{EvalS: 60, BuildS: 1800, ClosureBytes: 1 << 30}}, func(string) {})
	ne, ok := err.(*Error)
	if !ok || ne.Code != "closure_too_large" || !strings.Contains(ne.Message, "x-cuda") {
		t.Fatalf("cap: %v", err)
	}

	// Eval timeout via exit 124, and eval failure via fixture.
	r.Scripts = append([]shell.Script{{Prefix: []string{"timeout", "60", "nix", "eval"}, Result: shell.Result{ExitCode: 124}}}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r3", BaseRef: "abc123", Limits: Limits{EvalS: 60}}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "eval_failed" || ne.Message != "evaluation exceeded 60 s" {
		t.Fatalf("eval timeout: %v", err)
	}
	r.Scripts = append([]shell.Script{{Prefix: []string{"timeout", "60", "nix", "eval"}, Result: shell.Result{ExitCode: 1, Stderr: []byte(fixture(t, "syntax.stderr"))}}}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r4", BaseRef: "abc123", Limits: Limits{EvalS: 60}}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "eval_failed" || ne.FragmentLine != 1 {
		t.Fatalf("eval failure: %v", err)
	}
	// Missing base with no repo URL.
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r5", BaseRef: "nothere"}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "internal" || !strings.Contains(ne.Message, "base nothere unavailable") {
		t.Fatalf("missing base: %v", err)
	}
}

func TestCacheUnreachableDetection(t *testing.T) {
	if !CacheUnreachable("warning: error: unable to download 'https://cache.repose.herakraft.co/nar/x': Couldn't resolve host name (6); retrying in 300 ms\n") {
		t.Fatal("substituter failure not recognised")
	}
	if CacheUnreachable("copying path '/nix/store/x' from 'https://cache.nixos.org'...\n") {
		t.Fatal("false positive")
	}
}
