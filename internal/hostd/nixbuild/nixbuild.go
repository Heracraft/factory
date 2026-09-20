// Package nixbuild runs a project's fragment through the platform flake:
// write it, evaluate it under the restricted flags, build the derivation
// inside a bounded scope as an unprivileged user streaming the log, check
// the closure size, root the result. The flake contract hostd invokes is
// docs/interfaces/nix-build-contract.md; the policy (flags, limits, the
// error mapping in errors.go) is workstream 12's
// (docs/workstreams/12-nix-config-pipeline.md §5).
package nixbuild

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/heracraft/repose/internal/hostd/gcroot"
	"github.com/heracraft/repose/internal/hostd/shell"
)

// Limits are the caps the api sends with Build.
type Limits struct {
	EvalS        uint32
	BuildS       uint32
	Cores        uint32
	ClosureBytes uint64
}

// Request is one build.
type Request struct {
	ProjectID  string
	RevisionID string
	Fragment   []byte
	BaseRef    string
	Limits     Limits
}

// Result is a successful build.
type Result struct {
	SystemClosure string
	ClosureBytes  uint64
	Kernel        string
	Initrd        string
	// CacheUnreachable is set when Nix reported a substituter it could not
	// reach; the build fell back to source and hostd warns the api.
	CacheUnreachable bool
}

// CacheUnreachable recognises Nix's substituter failure lines.
func CacheUnreachable(stderr string) bool {
	return strings.Contains(stderr, "unable to download") || strings.Contains(stderr, "substituter") && strings.Contains(stderr, "failed") ||
		strings.Contains(stderr, "Couldn't resolve host") || strings.Contains(stderr, "Could not connect to server")
}

// Builder is what the guest Manager calls.
type Builder interface {
	Build(ctx context.Context, req Request, log func(line string)) (*Result, error)
	PathExists(ctx context.Context, path string) (bool, error)
}

// Info is what a system closure exposes for booting.
type Info struct {
	Kernel       string
	Initrd       string
	Init         string
	KernelParams string
}

// ClosureInfo reads kernel, initrd, init and kernel-params from a NixOS
// system closure.
func ClosureInfo(closure string) (Info, error) {
	var in Info
	var err error
	if in.Kernel, err = filepath.EvalSymlinks(filepath.Join(closure, "kernel")); err != nil {
		return in, fmt.Errorf("closure kernel: %w", err)
	}
	if in.Initrd, err = filepath.EvalSymlinks(filepath.Join(closure, "initrd")); err != nil {
		return in, fmt.Errorf("closure initrd: %w", err)
	}
	in.Init = filepath.Join(closure, "init")
	if _, err := os.Stat(in.Init); err != nil {
		return in, fmt.Errorf("closure init: %w", err)
	}
	b, err := os.ReadFile(filepath.Join(closure, "kernel-params"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return in, fmt.Errorf("closure kernel-params: %w", err)
	}
	in.KernelParams = strings.TrimSpace(string(b))
	return in, nil
}

// KernelChanged is the `kernel_changed` rule of the Build result: a new
// closure needs a reboot when its kernel or initrd is a different store
// path from the one the guest runs (docs/workstreams/12-nix-config-pipeline.md
// "Flags and limits"). A package-only change keeps both.
func KernelChanged(current, next Info) bool {
	return current.Kernel != next.Kernel || current.Initrd != next.Initrd
}

// Real runs nix through a shell.Runner.
type Real struct {
	R           shell.Runner
	BuildsDir   string // /var/lib/repose/builds
	BaseDir     string // /var/lib/repose/base
	BaseRepoURL string // cloned when a base checkout is missing; empty means it must exist
	// BaseSSHKey is a private key file for cloning BaseRepoURL over SSH;
	// empty means git's own configuration decides.
	BaseSSHKey string
	BaseSubdir string // nix
	// BaseScheme is how the checkout is named as a flake: "git"
	// (git+file://<checkout>?dir=nix, the contract: the flake reads files
	// above nix/) or "path" (path:<checkout>/nix, for a self-contained test
	// flake).
	BaseScheme    string
	EvalAttr      string // guestSystem.config.system.build.toplevel.drvPath
	Substituters  string
	MemoryMax     string // 16G
	Roots         gcroot.Roots
	KeepRevisions int
	// UseScope wraps eval and build in `systemd-run --scope` with CPUQuota,
	// MemoryMax and a RuntimeMaxSec backstop above the timeout.
	UseScope bool
	// User is the unprivileged account eval and build run as ("nixbuild"
	// on a host); empty runs as the caller (tests). Needs root and setpriv.
	User string
	// Home is that user's HOME (its nix cache lives there).
	Home string
	// Timeout is the timeout binary; "" disables the wrapper (tests).
	Timeout string
	// LogLines is how many lines of the failed builder's log go into a
	// build_failed message (200 in the workstream doc).
	LogLines int
}

// Defaults fills the documented values into zero fields.
func (b *Real) Defaults() *Real {
	if b.BaseSubdir == "" {
		b.BaseSubdir = "nix"
	}
	if b.BaseScheme == "" {
		b.BaseScheme = "git"
	}
	if b.EvalAttr == "" {
		b.EvalAttr = "guestSystem.config.system.build.toplevel.drvPath"
	}
	if b.Substituters == "" {
		// The platform overlay cache is a host option
		// (repose.host.overlayCache), passed as --substituters; the default
		// is the public cache alone.
		b.Substituters = "https://cache.nixos.org"
	}
	if b.MemoryMax == "" {
		b.MemoryMax = "16G"
	}
	if b.KeepRevisions == 0 {
		b.KeepRevisions = 3
	}
	if b.LogLines == 0 {
		b.LogLines = 200
	}
	if b.User != "" && b.Home == "" {
		b.Home = "/var/lib/repose/" + b.User
	}
	return b
}

// PathExists implements Builder.
func (b *Real) PathExists(ctx context.Context, path string) (bool, error) {
	_, err := b.R.Run(ctx, "nix", "path-info", path)
	var ee *shell.ExitError
	if errors.As(err, &ee) {
		return false, nil
	}
	return err == nil, err
}

func validRef(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/\\ ") && !strings.HasPrefix(s, ".")
}

// ensureBase returns the checkout directory for ref, cloning it when the
// repository URL is configured and the checkout is missing.
func (b *Real) ensureBase(ctx context.Context, ref string) (string, error) {
	if !validRef(ref) {
		return "", &Error{Code: "invalid_argument", Message: "base_ref must be a git revision"}
	}
	dir := filepath.Join(b.BaseDir, ref)
	if _, err := os.Stat(filepath.Join(dir, b.BaseSubdir, "flake.nix")); err == nil {
		return dir, nil
	}
	if b.BaseRepoURL == "" {
		return "", &Error{Code: "internal", Message: "base " + ref + " unavailable: no checkout under " + b.BaseDir}
	}
	if err := os.MkdirAll(b.BaseDir, 0o755); err != nil {
		return "", fmt.Errorf("base dir: %w", err)
	}
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp) // leftover from an interrupted clone
	clone := []string{"git", "clone", "--quiet", "--no-checkout", b.BaseRepoURL, tmp}
	if b.BaseSSHKey != "" {
		clone = append([]string{"env", "GIT_SSH_COMMAND=ssh -i " + b.BaseSSHKey + " -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new"}, clone...)
	}
	if _, err := b.R.Run(ctx, clone...); err != nil {
		return "", &Error{Code: "internal", Message: "base " + ref + " unavailable: clone failed: " + err.Error()}
	}
	if _, err := b.R.Run(ctx, "git", "-C", tmp, "checkout", "--quiet", ref); err != nil {
		_ = os.RemoveAll(tmp) // a bad ref; nothing to keep
		return "", &Error{Code: "internal", Message: "base " + ref + " unavailable: checkout failed: " + err.Error()}
	}
	if err := os.Rename(tmp, dir); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("base checkout: %w", err)
	}
	return dir, nil
}

// flakeRef names the checkout's nix/ directory as a flake.
func (b *Real) flakeRef(checkout string) string {
	if b.BaseScheme == "path" {
		return "path:" + filepath.Join(checkout, b.BaseSubdir)
	}
	return "git+file://" + checkout + "?dir=" + b.BaseSubdir
}

// wrap builds the command line around a nix invocation: the scope with its
// resource properties and the RuntimeMaxSec backstop, the user switch, the
// environment the unprivileged user needs, and the timeout.
func (b *Real) wrap(unit string, secs uint32, cores uint32, argv []string) []string {
	if b.Timeout != "" && secs > 0 {
		argv = append([]string{b.Timeout, "-k", "5", strconv.FormatUint(uint64(secs), 10)}, argv...)
	}
	if b.User != "" {
		argv = append([]string{
			"setpriv", "--reuid=" + b.User, "--regid=" + b.User, "--init-groups",
			"--bounding-set=-all", "--inh-caps=-all", "--no-new-privs", "--",
			"env", "HOME=" + b.Home, "USER=" + b.User, "LOGNAME=" + b.User, "NIX_REMOTE=daemon",
		}, argv...)
	}
	if b.UseScope {
		if cores == 0 {
			cores = 8
		}
		props := []string{"systemd-run", "--scope", "--quiet", "--unit", unit,
			"-p", fmt.Sprintf("CPUQuota=%d%%", cores*100),
			"-p", "MemoryMax=" + b.MemoryMax}
		if secs > 0 {
			props = append(props, "-p", "RuntimeMaxSec="+strconv.FormatUint(uint64(secs)+30, 10))
		}
		argv = append(append(props, "--"), argv...)
	}
	return argv
}

// isTimeout reports whether a failed command was stopped by the time cap:
// `timeout` exits 124, and the scope's RuntimeMaxSec backstop kills with
// SIGTERM (143) or SIGKILL (137) once the cap is past.
func isTimeout(err error, elapsed time.Duration, secs uint32) bool {
	var ee *shell.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	switch ee.Result.ExitCode {
	case 124:
		return true
	case 137, 143:
		return secs > 0 && elapsed >= time.Duration(secs)*time.Second
	}
	return false
}

// chownTree hands the build directory to the build user so the
// unprivileged eval can read the fragment.
func (b *Real) chownTree(dir string) error {
	if b.User == "" {
		return nil
	}
	u, err := user.Lookup(b.User)
	if err != nil {
		return fmt.Errorf("build user: %w", err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}

// Build implements Builder.
func (b *Real) Build(ctx context.Context, req Request, log func(string)) (*Result, error) {
	if !validRef(req.RevisionID) {
		return nil, &Error{Code: "invalid_argument", Message: "revision_id required"}
	}
	dir := filepath.Join(b.BuildsDir, req.RevisionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("build dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fragment.nix"), req.Fragment, 0o600); err != nil {
		return nil, fmt.Errorf("write fragment: %w", err)
	}
	if err := b.chownTree(dir); err != nil {
		return nil, err
	}
	checkout, err := b.ensureBase(ctx, req.BaseRef)
	if err != nil {
		return nil, err
	}
	allowed, err := AllowedURIs(filepath.Join(checkout, b.BaseSubdir, "flake.lock"))
	if err != nil {
		return nil, &Error{Code: "internal", Message: "base " + req.BaseRef + " unavailable: " + err.Error()}
	}
	allowed = append(allowed, "path:"+dir)
	unit := "repose-build-" + req.RevisionID

	log("evaluating configuration")
	evalArgv := b.wrap(unit+"-eval", req.Limits.EvalS, req.Limits.Cores, []string{
		"nix", "eval", "--raw", "--no-write-lock-file", "--show-trace",
		"--option", "restrict-eval", "true",
		"--option", "allow-import-from-derivation", "false",
		"--option", "pure-eval", "true",
		"--option", "eval-cache", "false",
		"--option", "allowed-uris", strings.Join(allowed, " "),
		"--max-call-depth", "10000",
		"--override-input", "fragment", "path:" + dir,
		b.flakeRef(checkout) + "#" + b.EvalAttr,
	})
	start := time.Now()
	res, err := b.R.Run(ctx, evalArgv...)
	if err != nil {
		if isTimeout(err, time.Since(start), req.Limits.EvalS) {
			return nil, EvalTimeout(req.Limits.EvalS)
		}
		var ee *shell.ExitError
		if errors.As(err, &ee) {
			return nil, MapEvalError(string(ee.Result.Stderr))
		}
		return nil, fmt.Errorf("nix eval: %w", err)
	}
	drv := strings.TrimSpace(string(res.Stdout))
	if !strings.HasPrefix(drv, "/nix/store/") || !strings.HasSuffix(drv, ".drv") {
		return nil, &Error{Code: "internal", Message: "nix eval did not return a derivation path: " + shell.Tail([]byte(drv), 200)}
	}

	log("building " + drvName(drv))
	buildArgv := b.wrap(unit, req.Limits.BuildS, req.Limits.Cores, []string{
		"nix", "build", "--no-link", "--print-out-paths", "--print-build-logs",
		"--option", "sandbox", "true",
		"--max-jobs", "1",
		"--cores", strconv.FormatUint(uint64(req.Limits.Cores), 10),
		"--option", "substituters", b.Substituters,
		drv + "^*",
	})
	start = time.Now()
	out, tail, err := b.stream(ctx, buildArgv, log)
	if err != nil {
		if isTimeout(err, time.Since(start), req.Limits.BuildS) {
			return nil, BuildTimeout(req.Limits.BuildS, tail)
		}
		var ee *shell.ExitError
		if errors.As(err, &ee) {
			e := MapBuildError(tail)
			if d := FailedDerivation(tail); d != "" {
				if l, lerr := b.R.Run(ctx, "nix", "log", d); lerr == nil {
					e.Message = e.Message + "\n\n" + drvName(d) + " log (last " + strconv.Itoa(b.LogLines) + " lines):\n" + lastLines(string(l.Stdout), b.LogLines)
				}
			}
			return nil, e
		}
		return nil, fmt.Errorf("nix build: %w", err)
	}
	outPath := ""
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(l, "/") {
			outPath = strings.TrimSpace(l)
		}
	}
	if outPath == "" {
		return nil, &Error{Code: "internal", Message: "nix build printed no output path"}
	}
	size, err := b.closureSize(ctx, outPath)
	if err != nil {
		return nil, err
	}
	if req.Limits.ClosureBytes > 0 && size > req.Limits.ClosureBytes {
		largest, _ := b.largestPaths(ctx, outPath, 10) // best effort detail for the message
		return nil, ClosureTooLarge(size, req.Limits.ClosureBytes, largest)
	}
	if err := b.Roots.Set(gcroot.RevisionRoot(req.ProjectID+"-"+req.RevisionID), outPath); err != nil {
		return nil, err
	}
	if _, err := b.Roots.PruneRevisions(req.ProjectID, b.KeepRevisions); err != nil {
		return nil, err
	}
	info, err := ClosureInfo(outPath)
	if err != nil {
		return nil, &Error{Code: "internal", Message: "built closure is not a bootable system: " + err.Error()}
	}
	log("built " + outPath)
	return &Result{SystemClosure: outPath, ClosureBytes: size, Kernel: info.Kernel, Initrd: info.Initrd, CacheUnreachable: CacheUnreachable(tail)}, nil
}

// stream runs argv, feeding stderr lines to log; it returns stdout, the
// last 32 KB of stderr, and the exit error.
func (b *Real) stream(ctx context.Context, argv []string, log func(string)) (string, string, error) {
	p, err := b.R.Start(ctx, nil, argv...)
	if err != nil {
		return "", "", err
	}
	var wg sync.WaitGroup
	var out strings.Builder
	tail := newTail(MessageCap)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&out, p.Stdout()) // a copy error surfaces as a missing out path below
	}()
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(p.Stderr())
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			tail.Write(line + "\n")
			log(line)
		}
	}()
	wg.Wait()
	err = p.Wait()
	return out.String(), tail.String(), err
}

type tailBuf struct {
	max int
	buf []byte
}

func newTail(max int) *tailBuf { return &tailBuf{max: max} }

func (t *tailBuf) Write(s string) {
	t.buf = append(t.buf, s...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
}

func (t *tailBuf) String() string { return string(t.buf) }

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func (b *Real) closureSize(ctx context.Context, path string) (uint64, error) {
	res, err := b.R.Run(ctx, "nix", "path-info", "-S", path)
	if err != nil {
		return 0, fmt.Errorf("nix path-info: %w", err)
	}
	f := strings.Fields(string(res.Stdout))
	if len(f) < 2 {
		return 0, fmt.Errorf("nix path-info: unexpected output %q", shell.Tail(res.Stdout, 200))
	}
	return strconv.ParseUint(f[len(f)-1], 10, 64)
}

func (b *Real) largestPaths(ctx context.Context, path string, n int) (string, error) {
	res, err := b.R.Run(ctx, "nix", "path-info", "-rs", path)
	if err != nil {
		return "", err
	}
	type row struct {
		p string
		s uint64
	}
	var rows []row
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		s, err := strconv.ParseUint(f[len(f)-1], 10, 64)
		if err != nil {
			continue
		}
		rows = append(rows, row{f[0], s})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].s > rows[j].s })
	if len(rows) > n {
		rows = rows[:n]
	}
	var sb strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&sb, "  %s  %s\n", humanBytes(r.s), r.p)
	}
	return sb.String(), nil
}

// humanBytes prints sizes the way the messages in the workstream doc do:
// "31.2 GB", "20 GB", "512.0 MB".
func humanBytes(n uint64) string {
	const gb = 1 << 30
	const mb = 1 << 20
	switch {
	case n >= gb:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/gb), ".0") + " GB"
	case n >= mb:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/mb), ".0") + " MB"
	}
	return fmt.Sprintf("%d B", n)
}

// Fake is the Builder for state machine tests.
type Fake struct {
	mu           sync.Mutex
	Closure      string
	ClosureBytes uint64
	Kernel       string
	Initrd       string
	Delay        time.Duration
	Fail         *Error
	Lines        []string
	Exists       map[string]bool
	Calls        []Request
}

// Build implements Builder.
func (f *Fake) Build(ctx context.Context, req Request, log func(string)) (*Result, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, req)
	fail, lines, delay := f.Fail, f.Lines, f.Delay
	f.mu.Unlock()
	for _, l := range lines {
		log(l)
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail != nil {
		return nil, fail
	}
	return &Result{SystemClosure: f.Closure, ClosureBytes: f.ClosureBytes, Kernel: f.Kernel, Initrd: f.Initrd}, nil
}

// PathExists implements Builder.
func (f *Fake) PathExists(_ context.Context, path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Exists == nil {
		return true, nil
	}
	return f.Exists[path], nil
}
