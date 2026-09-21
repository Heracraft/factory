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

// The fixtures are real `nix eval` stderr from the platform flake
// (docs/interfaces/nix-build-contract.md "Fixtures"); the first lines are
// the canonical ones from docs/workstreams/12-nix-config-pipeline.md.
func TestMapEvalErrorFixtures(t *testing.T) {
	cases := []struct {
		file, wantFirst string
		wantLine        int32
	}{
		{"syntax.stderr", "syntax error at fragment.nix:1:49, unexpected ';'", 1},
		{"missing.stderr", "attribute 'ripgrepp' missing at fragment.nix:1:36 (did you mean one of ripgrep, ipgrep or repgrep?)", 1},
		{"fetch.stderr", "eval-time fetch not allowed at fragment.nix:1:39; use pkgs.fetchurl { url = ...; hash = ...; }", 1},
		{"fetchgit.stderr", "eval-time fetch not allowed at fragment.nix:1:39; use pkgs.fetchgit or pkgs.fetchurl with a hash instead of builtins.fetch*", 1},
		{"abspath.stderr", "access to absolute path '/etc/passwd' is forbidden in pure evaluation mode (use '--impure' to override) at fragment.nix:1:37; a fragment may only read files it carries", 1},
		{"nixpath.stderr", "<nixpkgs> is not available at fragment.nix:1:44; use the pkgs argument, which is the platform's pinned nixpkgs", 1},
		{"ifd.stderr", "import-from-derivation is not allowed at fragment.nix:1:37; a fragment cannot import a file that a build produces", 1},
		{"option.stderr", "option 'services.postgresql' does not exist in a fragment; system services come from the menu or `repose config menu`", 0},
		{"assertion.stderr", "repose.system: option 'networking.firewall' is not allowed in a fragment; system services come from the menu or `repose config menu` (allowed: services.postgresql, services.redis, services.mysql, services.memcached, services.rabbitmq, services.meilisearch, services.nats)", 0},
		{"hmoverlays.stderr", "fragment: nixpkgs.overlays is ignored with useGlobalPkgs; use repose.overlays = [ ... ] instead", 0},
	}
	for _, c := range cases {
		e := MapEvalError(fixture(t, c.file))
		if e.Code != "eval_failed" {
			t.Errorf("%s: code %s", c.file, e.Code)
		}
		if got := firstLine(e.Message); got != c.wantFirst {
			t.Errorf("%s: first line\n got %q\nwant %q", c.file, got, c.wantFirst)
		}
		if e.FragmentLine != c.wantLine {
			t.Errorf("%s: fragment_line %d, want %d", c.file, e.FragmentLine, c.wantLine)
		}
		// The verbatim block follows a blank line and ends with Nix's own
		// final error; a trace over 32 KB loses its head, never its tail.
		if i := strings.Index(e.Message, "\n\n"); i < 0 || !strings.Contains(e.Message[i:], "error:") || len(e.Message) > MessageCap+len(c.wantFirst)+2 {
			t.Errorf("%s: verbatim output missing or over the cap (%d bytes)", c.file, len(e.Message))
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
	if FailedDerivation(fixture(t, "build.stderr")) != "/nix/store/6v3cd4dy8pfmi24r2bw1hipqsk8wchgb-fails-1.0.drv" {
		t.Fatal("failed derivation not found")
	}
	bt := BuildTimeout(1800, fixture(t, "build.stderr"))
	if bt.Code != "build_timeout" || firstLine(bt.Message) != "build timed out after 30 minutes while building fails-1.0" {
		t.Fatalf("timeout: %s %q", bt.Code, firstLine(bt.Message))
	}
	if got := firstLine(BuildTimeout(5, "building '/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-sleep-forever-1.0.drv'...\n").Message); got != "build timed out after 5 s while building sleep-forever-1.0" {
		t.Fatalf("short timeout: %q", got)
	}
	if got := BuildTimeout(1800, "").Message; got != "build timed out after 30 minutes\n\n" {
		t.Fatalf("timeout without derivation: %q", got)
	}
	et := EvalTimeout(60)
	if et.Code != "eval_failed" || et.Message != "evaluation exceeded 60 s" {
		t.Fatalf("eval timeout: %v", et)
	}
	ct := ClosureTooLarge(33501750067, 20<<30, "  30 GB  /nix/store/x-cuda\n")
	if firstLine(ct.Message) != "closure is 31.2 GB, limit is 20 GB; largest paths:" || !strings.HasSuffix(ct.Message, "/nix/store/x-cuda") {
		t.Fatalf("closure: %q", ct.Message)
	}
}

func TestKernelChanged(t *testing.T) {
	a := Info{Kernel: "/nix/store/k1-linux-6.17.4/bzImage", Initrd: "/nix/store/i1-initrd/initrd"}
	pkgOnly := Info{Kernel: a.Kernel, Initrd: a.Initrd}
	newKernel := Info{Kernel: "/nix/store/k2-linux-6.17.5/bzImage", Initrd: a.Initrd}
	newInitrd := Info{Kernel: a.Kernel, Initrd: "/nix/store/i2-initrd/initrd"}
	if KernelChanged(a, pkgOnly) {
		t.Fatal("package-only change reported kernel_changed")
	}
	if !KernelChanged(a, newKernel) || !KernelChanged(a, newInitrd) {
		t.Fatal("kernel or initrd change not reported")
	}
}

func TestAllowedURIsFromLock(t *testing.T) {
	got, err := AllowedURIs(filepath.Join("..", "..", "..", "nix", "flake.lock"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{"github:NixOS/nixpkgs/", "github:nix-community/home-manager/", "github:microvm-nix/microvm.nix/", "?narHash=sha256-"} {
		if !strings.Contains(joined, want) {
			t.Errorf("allowed-uris missing %q in %q", want, joined)
		}
	}
	for _, u := range got {
		if strings.Contains(u, "fragment-placeholder") || strings.Contains(u, "%2F") || strings.Contains(u, "=") && !strings.Contains(u, "%3D") {
			t.Errorf("unexpected entry %q", u)
		}
	}
}

func TestWrapArgv(t *testing.T) {
	b := (&Real{Timeout: "timeout", UseScope: true, User: "nixbuild"}).Defaults()
	got := strings.Join(b.wrap("repose-build-r1", 1800, 8, []string{"nix", "build"}), " ")
	want := "systemd-run --scope --quiet --unit repose-build-r1 -p CPUQuota=800% -p MemoryMax=16G -p RuntimeMaxSec=1830 -- setpriv --reuid=nixbuild --regid=nixbuild --init-groups --bounding-set=-all --inh-caps=-all --no-new-privs -- env HOME=/var/lib/repose/nixbuild USER=nixbuild LOGNAME=nixbuild NIX_REMOTE=daemon timeout -k 5 1800 nix build"
	if got != want {
		t.Fatalf("wrap:\n got %s\nwant %s", got, want)
	}
	plain := (&Real{}).Defaults()
	if got := strings.Join(plain.wrap("u", 60, 8, []string{"nix", "eval"}), " "); got != "nix eval" {
		t.Fatalf("plain wrap: %s", got)
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

func fakeBase(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	_ = os.MkdirAll(filepath.Join(base, "abc123", "nix"), 0o755)
	_ = os.WriteFile(filepath.Join(base, "abc123", "nix", "flake.nix"), []byte("{}"), 0o644)
	lock := `{"nodes":{"nixpkgs":{"locked":{"type":"github","owner":"NixOS","repo":"nixpkgs","rev":"b1b8","narHash":"sha256-zVx="}},"root":{"inputs":{"nixpkgs":"nixpkgs"}}},"root":"root","version":7}`
	_ = os.WriteFile(filepath.Join(base, "abc123", "nix", "flake.lock"), []byte(lock), 0o644)
	return base
}

func TestRealBuildFlow(t *testing.T) {
	closure := fakeClosure(t)
	base := fakeBase(t)
	r := &shell.Fake{Scripts: []shell.Script{
		{Prefix: []string{"timeout", "-k", "5", "60", "nix", "eval"}, Result: shell.Result{Stdout: []byte("/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-guest.drv\n")}},
		{Prefix: []string{"timeout", "-k", "5", "1800", "nix", "build"}, Result: shell.Result{Stdout: []byte(closure + "\n"), Stderr: []byte("building '/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-guest.drv'...\ncopying path\n")}},
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
	evalCall := strings.Join(r.CallsWithPrefix("timeout", "-k", "5", "60", "nix", "eval")[0], " ")
	for _, want := range []string{
		"--override-input fragment path:" + filepath.Join(b.BuildsDir, "r1"),
		"--option restrict-eval true", "--option pure-eval true", "--option allow-import-from-derivation false",
		"--option allowed-uris github:NixOS/nixpkgs/b1b8?narHash=sha256-zVx%3D path:" + filepath.Join(b.BuildsDir, "r1"),
		"git+file://" + filepath.Join(base, "abc123") + "?dir=nix#guestSystem.config.system.build.toplevel.drvPath",
	} {
		if !strings.Contains(evalCall, want) {
			t.Fatalf("eval argv missing %q: %s", want, evalCall)
		}
	}
	buildCall := strings.Join(r.CallsWithPrefix("timeout", "-k", "5", "1800", "nix", "build")[0], " ")
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
		{Prefix: []string{"nix", "path-info", "-rs"}, Result: shell.Result{Stdout: []byte("/nix/store/x-cuda\t30000000000\n/nix/store/y-glibc\t30000000\n")}},
	}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r2", BaseRef: "abc123", Limits: Limits{EvalS: 60, BuildS: 1800, ClosureBytes: 1 << 30}}, func(string) {})
	ne, ok := err.(*Error)
	if !ok || ne.Code != "closure_too_large" || !strings.Contains(ne.Message, "x-cuda") || firstLine(ne.Message) != "closure is 5 GB, limit is 1 GB; largest paths:" {
		t.Fatalf("cap: %v", err)
	}
	if _, err := b.Roots.Get("rev-p1-r2"); err == nil {
		t.Fatal("an over-cap closure must not be rooted")
	}

	// Eval timeout via exit 124, and eval failure via fixture.
	r.Scripts = append([]shell.Script{{Prefix: []string{"timeout", "-k", "5", "60", "nix", "eval"}, Result: shell.Result{ExitCode: 124}}}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r3", BaseRef: "abc123", Limits: Limits{EvalS: 60}}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "eval_failed" || ne.Message != "evaluation exceeded 60 s" {
		t.Fatalf("eval timeout: %v", err)
	}
	r.Scripts = append([]shell.Script{{Prefix: []string{"timeout", "-k", "5", "60", "nix", "eval"}, Result: shell.Result{ExitCode: 1, Stderr: []byte(fixture(t, "syntax.stderr"))}}}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r4", BaseRef: "abc123", Limits: Limits{EvalS: 60}}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "eval_failed" || ne.FragmentLine != 1 {
		t.Fatalf("eval failure: %v", err)
	}
	// Build failure pulls the builder's log through `nix log`.
	r.Scripts = append([]shell.Script{
		{Prefix: []string{"timeout", "-k", "5", "60", "nix", "eval"}, Result: shell.Result{Stdout: []byte("/nix/store/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-nixos-system-guest.drv\n")}},
		{Prefix: []string{"timeout", "-k", "5", "1800", "nix", "build"}, Result: shell.Result{ExitCode: 1, Stderr: []byte(fixture(t, "build.stderr"))}},
		{Prefix: []string{"nix", "log", "/nix/store/6v3cd4dy8pfmi24r2bw1hipqsk8wchgb-fails-1.0.drv"}, Result: shell.Result{Stdout: []byte("compiling\nboom\n")}},
	}, r.Scripts...)
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r6", BaseRef: "abc123", Limits: Limits{EvalS: 60, BuildS: 1800}}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "build_failed" || firstLine(ne.Message) != "build of fails-1.0 failed" || !strings.Contains(ne.Message, "fails-1.0 log (last 200 lines):\ncompiling\nboom") {
		t.Fatalf("build failure: %v", err)
	}
	// Missing base with no repo URL.
	_, err = b.Build(context.Background(), Request{ProjectID: "p1", RevisionID: "r5", BaseRef: "nothere"}, func(string) {})
	if ne, ok := err.(*Error); !ok || ne.Code != "internal" || !strings.Contains(ne.Message, "base nothere unavailable") {
		t.Fatalf("missing base: %v", err)
	}
}

func TestEnsureBaseClonesWithKey(t *testing.T) {
	base := t.TempDir()
	r := &shell.Fake{Scripts: []shell.Script{
		{Prefix: []string{"env"}, Handle: func(argv []string) (shell.Result, error) {
			// Simulate the clone by creating the checkout the way git would.
			dst := argv[len(argv)-1]
			_ = os.MkdirAll(filepath.Join(dst, "nix"), 0o755)
			_ = os.WriteFile(filepath.Join(dst, "nix", "flake.nix"), []byte("{}"), 0o644)
			return shell.Result{}, nil
		}},
	}}
	b := (&Real{R: r, BaseDir: base, BaseRepoURL: "git@github.com:heracraft/repose.git", BaseSSHKey: "/var/lib/repose/hostd/base-deploy-key", User: "nixbuild"}).Defaults()
	dir, err := b.ensureBase(context.Background(), "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(base, "deadbeef") {
		t.Fatalf("dir %s", dir)
	}
	// The clone is root's; the evaluation runs as the build user, whose
	// libgit2 refuses a repository it does not own (I-93).
	if got := r.CallsWithPrefix("chown"); len(got) != 1 || strings.Join(got[0], " ") != "chown -R nixbuild: "+dir {
		t.Fatalf("chown calls: %v", got)
	}
	// An existing checkout (placed by hand) is handed over the same way.
	if _, err := b.ensureBase(context.Background(), "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if got := r.CallsWithPrefix("chown"); len(got) != 2 {
		t.Fatalf("chown on an existing checkout: %v", got)
	}
	call := strings.Join(r.CallsWithPrefix("env")[0], " ")
	if !strings.Contains(call, "GIT_SSH_COMMAND=ssh -i /var/lib/repose/hostd/base-deploy-key") || !strings.Contains(call, "git clone --quiet --no-checkout git@github.com:heracraft/repose.git") {
		t.Fatalf("clone argv: %s", call)
	}
	if got := strings.Join(r.CallsWithPrefix("git", "-C")[0], " "); !strings.HasSuffix(got, "checkout --quiet deadbeef") {
		t.Fatalf("checkout argv: %s", got)
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

// Build.base_version lands beside the fragment as `base-version`, a bad
// label is refused, and an empty one removes a stale file (I-118).
func TestWriteBaseVersion(t *testing.T) {
	dir := t.TempDir()
	if err := writeBaseVersion(dir, "2026.09.20.3"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "base-version"))
	if err != nil || string(b) != "2026.09.20.3\n" {
		t.Fatalf("base-version file: %q %v", b, err)
	}
	if err := writeBaseVersion(dir, "../etc"); err == nil {
		t.Fatal("a label with a slash was accepted")
	}
	if err := writeBaseVersion(dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "base-version")); !os.IsNotExist(err) {
		t.Fatalf("empty label left the file: %v", err)
	}
}
