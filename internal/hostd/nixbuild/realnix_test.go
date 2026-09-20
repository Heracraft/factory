package nixbuild

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/hostd/gcroot"
	"github.com/heracraft/repose/internal/hostd/shell"
)

// The real-Nix corpus (docs/workstreams/12-nix-config-pipeline.md §7): the
// five canonical cases and the restrict-eval checks run through the exact
// command lines of docs/interfaces/nix-build-contract.md against
// testdata/miniflake, which has the same `fragment` input and
// `guestSystem` output as the platform flake and no nixpkgs, so every case
// runs offline in seconds. Case (c) has the timeout lowered to 5 s and
// case (d) the cap to 100 MB, as the doc says. Runs when nix is on PATH and
// REPOSE_NIX_TESTS=1 (the CI nix job sets it; the go job has no nix).

func realNix(t *testing.T) *Real {
	t.Helper()
	if os.Getenv("REPOSE_NIX_TESTS") == "" {
		t.Skip("set REPOSE_NIX_TESTS=1 to run the real-nix corpus")
	}
	if _, err := exec.LookPath("nix"); err != nil {
		t.Skip("nix not on PATH")
	}
	src, err := filepath.Abs(filepath.Join("testdata", "miniflake"))
	if err != nil {
		t.Fatal(err)
	}
	// The checkout layout hostd expects: <base>/<ref>/nix/flake.nix.
	base := t.TempDir()
	dst := filepath.Join(base, "testref", "nix")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-r", src, dst).CombinedOutput(); err != nil {
		t.Fatalf("copy flake: %v: %s", err, out)
	}
	roots := filepath.Join(t.TempDir(), "gcroots")
	return (&Real{
		R: shell.Exec{}, BuildsDir: filepath.Join(t.TempDir(), "builds"), BaseDir: base,
		BaseScheme: "path", Roots: gcroot.Roots{Dir: roots}, Timeout: "timeout",
		Substituters: "https://cache.nixos.org", // the platform cache does not exist yet
	}).Defaults()
}

func realBuild(t *testing.T, b *Real, rev, fragment string, lim Limits) (*Result, *Error, []string) {
	t.Helper()
	var lines []string
	res, err := b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: rev, Fragment: []byte(fragment), BaseRef: "testref", Limits: lim},
		func(l string) { lines = append(lines, l) })
	if err == nil {
		return res, nil, lines
	}
	var ne *Error
	if !errors.As(err, &ne) {
		t.Fatalf("%s: unexpected error type: %v", rev, err)
	}
	return nil, ne, lines
}

var realLimits = Limits{EvalS: 60, BuildS: 300, Cores: 2, ClosureBytes: 100 << 20}

// 120 MB of zeros plus ripgrep's closure, against the 100 MB test cap.
var closureFirstLine = regexp.MustCompile(`^closure is 1[0-9]{2}(\.[0-9])? MB, limit is 100 MB; largest paths:$`)

func TestRealNixCanonicalCases(t *testing.T) {
	b := realNix(t)
	cases := []struct {
		name, fragment, code, first string
		line                        int32
	}{
		{"a-syntax", `{ pkgs, ... }: { home.packages = [ pkgs.ripgrep ; }`, "eval_failed", "syntax error at fragment.nix:1:49, unexpected ';'", 1},
		{"b-missing", `{ pkgs, ... }: { home.packages = [ pkgs.ripgrepp ]; }`, "eval_failed", "attribute 'ripgrepp' missing at fragment.nix:1:36 (did you mean one of ripgrep, ipgrep or repgrep?)", 1},
		{"e-fetch", `{ pkgs, ... }: { home.file.x.source = builtins.fetchurl "https://example.com/x"; }`, "eval_failed", "eval-time fetch not allowed at fragment.nix:1:39; use pkgs.fetchurl { url = ...; hash = ...; }", 1},
		{"readfile", `{ pkgs, ... }: { home.file.x.text = builtins.readFile "/etc/passwd"; }`, "eval_failed", "access to absolute path '/etc/passwd' is forbidden in pure evaluation mode (use '--impure' to override) at fragment.nix:1:37; a fragment may only read files it carries", 1},
		{"nixpath", `{ pkgs, ... }: { home.packages = [ (import <nixpkgs> {}).hello ]; }`, "eval_failed", "<nixpkgs> is not available at fragment.nix:1:44; use the pkgs argument, which is the platform's pinned nixpkgs", 1},
		{"build-fails", `{ pkgs, ... }: { home.packages = [ pkgs.fails ]; }`, "build_failed", "build of fails-1.0 failed", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ne, _ := realBuild(t, b, c.name, c.fragment, realLimits)
			if ne == nil {
				t.Fatalf("expected %s, build succeeded", c.code)
			}
			t.Logf("%s: %s: %s (fragment_line %d)", c.name, ne.Code, firstLine(ne.Message), ne.FragmentLine)
			if ne.Code != c.code || firstLine(ne.Message) != c.first || ne.FragmentLine != c.line {
				t.Fatalf("got %s %q line %d\nwant %s %q line %d\n--- verbatim ---\n%s", ne.Code, firstLine(ne.Message), ne.FragmentLine, c.code, c.first, c.line, ne.Message)
			}
			if c.code == "build_failed" && !strings.Contains(ne.Message, "fails-1.0 log (last 200 lines):") {
				t.Fatalf("builder log missing from the message:\n%s", ne.Message)
			}
		})
	}
}

// Case (c): a derivation that sleeps 31 minutes, with the cap lowered to 5 s.
func TestRealNixBuildTimeout(t *testing.T) {
	b := realNix(t)
	lim := realLimits
	lim.BuildS = 5
	start := time.Now()
	_, ne, _ := realBuild(t, b, "c-timeout", `{ pkgs, ... }: { home.packages = [ pkgs.sleep-forever ]; }`, lim)
	if ne == nil {
		t.Fatal("build succeeded")
	}
	t.Logf("c: %s: %s (after %s)", ne.Code, firstLine(ne.Message), time.Since(start).Round(time.Second))
	if ne.Code != "build_timeout" || firstLine(ne.Message) != "build timed out after 5 s while building sleep-forever-1.0" {
		t.Fatalf("got %s %q", ne.Code, firstLine(ne.Message))
	}
}

// Case (d): a 120 MB output against a 100 MB cap; no GC root afterwards.
func TestRealNixClosureCap(t *testing.T) {
	b := realNix(t)
	_, ne, _ := realBuild(t, b, "d-closure", `{ pkgs, ... }: { home.packages = [ pkgs.big pkgs.ripgrep ]; }`, realLimits)
	if ne == nil {
		t.Fatal("build succeeded")
	}
	t.Logf("d: %s:\n%s", ne.Code, ne.Message)
	if ne.Code != "closure_too_large" || !closureFirstLine.MatchString(firstLine(ne.Message)) {
		t.Fatalf("got %s %q", ne.Code, firstLine(ne.Message))
	}
	if !strings.Contains(ne.Message, "-big-1.0") || !strings.Contains(ne.Message, "-nixos-system-repose-guest-test") {
		t.Fatalf("largest paths not listed:\n%s", ne.Message)
	}
	if es, _ := b.Roots.List(); len(es) != 0 {
		t.Fatalf("an over-cap build left roots: %v", es)
	}
}

// A trivial fragment builds, is rooted, streams its log, and exposes the
// boot files; then kernel_changed across three revisions.
func TestRealNixSuccessRootAndKernelChanged(t *testing.T) {
	b := realNix(t)
	res, ne, lines := realBuild(t, b, "ok-1", `{ pkgs, ... }: { home.packages = [ pkgs.ripgrep ]; }`, realLimits)
	if ne != nil {
		t.Fatalf("build failed: %s", ne.Message)
	}
	t.Logf("ok-1: %s (%d bytes)\nlog: %s", res.SystemClosure, res.ClosureBytes, strings.Join(lines, " | "))
	if !strings.HasPrefix(res.SystemClosure, "/nix/store/") || res.ClosureBytes == 0 || !strings.Contains(res.Kernel, "kernel-6.17.4") {
		t.Fatalf("result %+v", res)
	}
	if got, _ := b.Roots.Get("rev-p1-ok-1"); got != res.SystemClosure {
		t.Fatalf("gc root: %q", got)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "evaluating configuration") || !strings.Contains(joined, "building nixos-system-repose-guest-test") || !strings.Contains(joined, "built /nix/store/") {
		t.Fatalf("log lines: %v", lines)
	}
	if fi, err := os.Stat(filepath.Join(b.BuildsDir, "ok-1", "fragment.nix")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("fragment file: %v %v", fi, err)
	}
	if ok, err := b.PathExists(context.Background(), res.SystemClosure); err != nil || !ok {
		t.Fatalf("PathExists: %v %v", ok, err)
	}
	cur, err := ClosureInfo(res.SystemClosure)
	if err != nil {
		t.Fatal(err)
	}

	// Package-only change: same kernel and initrd.
	res2, ne, _ := realBuild(t, b, "ok-2", `{ pkgs, ... }: { home.packages = [ pkgs.ripgrep pkgs.hello ]; }`, realLimits)
	if ne != nil {
		t.Fatalf("ok-2: %s", ne.Message)
	}
	next, _ := ClosureInfo(res2.SystemClosure)
	t.Logf("ok-2 (package-only): kernel_changed=%v", KernelChanged(cur, next))
	if KernelChanged(cur, next) || res2.SystemClosure == res.SystemClosure {
		t.Fatalf("package-only change: kernel_changed=%v, closure %s vs %s", KernelChanged(cur, next), res2.SystemClosure, res.SystemClosure)
	}
	// Kernel bump: a different kernel store path.
	res3, ne, _ := realBuild(t, b, "ok-3", `{ pkgs, ... }: { home.packages = [ pkgs.ripgrep ]; repose.testKernel = "6.17.5"; }`, realLimits)
	if ne != nil {
		t.Fatalf("ok-3: %s", ne.Message)
	}
	next3, _ := ClosureInfo(res3.SystemClosure)
	t.Logf("ok-3 (kernel bump): kernel_changed=%v (%s -> %s)", KernelChanged(cur, next3), filepath.Base(cur.Kernel), filepath.Base(next3.Kernel))
	if !KernelChanged(cur, next3) {
		t.Fatal("kernel bump not detected")
	}
	// The newest three revisions stay rooted.
	es, _ := b.Roots.List()
	if len(es) != 3 {
		t.Fatalf("roots: %v", es)
	}
}

// Fixed-output fetch with a hash builds with network inside the sandbox.
// Needs the network, so it is a separate opt-in (REPOSE_NIX_NETWORK=1).
func TestRealNixFixedOutputFetch(t *testing.T) {
	b := realNix(t)
	if os.Getenv("REPOSE_NIX_NETWORK") == "" {
		t.Skip("set REPOSE_NIX_NETWORK=1 to run the fetch case (needs network)")
	}
	frag := `{ pkgs, ... }: {
  home.packages = [ (pkgs.fetchurl {
    url = "https://raw.githubusercontent.com/NixOS/nixpkgs/b1b875982b17dabde9b4a37f3e229e74913e6db3/COPYING";
    hash = "sha256-yc8GUKaCC1iflqkgYODrk3ECuAj0jNXj813LpEnqCkE=";
  }) ];
}`
	res, ne, lines := realBuild(t, b, "fetch-ok", frag, realLimits)
	if ne != nil {
		t.Fatalf("fetch: %s", ne.Message)
	}
	t.Logf("fetched: %s\n%s", res.SystemClosure, strings.Join(lines, "\n"))
}
