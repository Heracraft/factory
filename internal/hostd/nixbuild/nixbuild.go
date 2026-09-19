// Package nixbuild runs a project's fragment through the platform flake:
// write it, evaluate it under the restricted flags, build the derivation
// inside a bounded scope streaming the log, check the closure size, root
// the result. The flake contract hostd invokes is
// docs/interfaces/nix-build-contract.md.
package nixbuild

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// Real runs nix through a shell.Runner.
type Real struct {
	R             shell.Runner
	BuildsDir     string // /var/lib/repose/builds
	BaseDir       string // /var/lib/repose/base
	BaseRepoURL   string // cloned when a base checkout is missing; empty means it must exist
	BaseSubdir    string // nix
	EvalAttr      string // guestSystem.config.system.build.toplevel.drvPath
	Substituters  string
	MemoryMax     string // 16G
	Roots         gcroot.Roots
	KeepRevisions int
	UseScope      bool
	Timeout       string // the timeout binary; "" disables the wrapper (tests)
}

// Defaults fills the documented values into zero fields.
func (b *Real) Defaults() *Real {
	if b.BaseSubdir == "" {
		b.BaseSubdir = "nix"
	}
	if b.EvalAttr == "" {
		b.EvalAttr = "guestSystem.config.system.build.toplevel.drvPath"
	}
	if b.Substituters == "" {
		b.Substituters = "https://cache.nixos.org https://cache.repose.herakraft.co"
	}
	if b.MemoryMax == "" {
		b.MemoryMax = "16G"
	}
	if b.KeepRevisions == 0 {
		b.KeepRevisions = 3
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

func (b *Real) ensureBase(ctx context.Context, ref string) (string, error) {
	if ref == "" || strings.ContainsAny(ref, "/\\ ") || strings.HasPrefix(ref, ".") {
		return "", &Error{Code: "invalid_argument", Message: "base_ref must be a git revision"}
	}
	dir := filepath.Join(b.BaseDir, ref)
	flake := filepath.Join(dir, b.BaseSubdir)
	if _, err := os.Stat(filepath.Join(flake, "flake.nix")); err == nil {
		return flake, nil
	}
	if b.BaseRepoURL == "" {
		return "", &Error{Code: "internal", Message: "base " + ref + " unavailable: no checkout under " + b.BaseDir}
	}
	tmp := dir + ".tmp"
	_ = os.RemoveAll(tmp) // leftover from an interrupted clone
	if _, err := b.R.Run(ctx, "git", "clone", "--quiet", "--no-checkout", b.BaseRepoURL, tmp); err != nil {
		return "", &Error{Code: "internal", Message: "base " + ref + " unavailable: clone failed: " + err.Error()}
	}
	if _, err := b.R.Run(ctx, "git", "-C", tmp, "checkout", "--quiet", ref); err != nil {
		_ = os.RemoveAll(tmp) // a bad ref; nothing to keep
		return "", &Error{Code: "internal", Message: "base " + ref + " unavailable: checkout failed: " + err.Error()}
	}
	if err := os.Rename(tmp, dir); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("base checkout: %w", err)
	}
	return flake, nil
}

func (b *Real) withTimeout(secs uint32, argv []string) []string {
	if b.Timeout == "" || secs == 0 {
		return argv
	}
	return append([]string{b.Timeout, strconv.FormatUint(uint64(secs), 10)}, argv...)
}

func isTimeout(err error) bool {
	var ee *shell.ExitError
	return errors.As(err, &ee) && ee.Result.ExitCode == 124
}

// Build implements Builder.
func (b *Real) Build(ctx context.Context, req Request, log func(string)) (*Result, error) {
	if req.RevisionID == "" || strings.ContainsAny(req.RevisionID, "/\\ ") || strings.HasPrefix(req.RevisionID, ".") {
		return nil, &Error{Code: "invalid_argument", Message: "revision_id required"}
	}
	dir := filepath.Join(b.BuildsDir, req.RevisionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("build dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fragment.nix"), req.Fragment, 0o600); err != nil {
		return nil, fmt.Errorf("write fragment: %w", err)
	}
	flake, err := b.ensureBase(ctx, req.BaseRef)
	if err != nil {
		return nil, err
	}
	log("evaluating configuration")
	evalArgv := b.withTimeout(req.Limits.EvalS, []string{
		"nix", "eval", "--raw", "--no-write-lock-file",
		"--option", "restrict-eval", "true",
		"--option", "allow-import-from-derivation", "false",
		"--option", "pure-eval", "true",
		"--option", "eval-cache", "false",
		"--max-call-depth", "10000",
		"--override-input", "fragment", "path:" + dir,
		"path:" + flake + "#" + b.EvalAttr,
	})
	res, err := b.R.Run(ctx, evalArgv...)
	if err != nil {
		if isTimeout(err) {
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
	buildArgv := []string{
		"nix", "build", "--no-link", "--print-out-paths", "--print-build-logs",
		"--option", "sandbox", "true",
		"--max-jobs", "1",
		"--cores", strconv.FormatUint(uint64(req.Limits.Cores), 10),
		"--option", "substituters", b.Substituters,
		drv + "^*",
	}
	buildArgv = b.withTimeout(req.Limits.BuildS, buildArgv)
	if b.UseScope {
		buildArgv = append([]string{"systemd-run", "--scope", "--quiet",
			"-p", fmt.Sprintf("CPUQuota=%d%%", req.Limits.Cores*100),
			"-p", "MemoryMax=" + b.MemoryMax, "--"}, buildArgv...)
	}
	out, tail, err := b.stream(ctx, buildArgv, log)
	if err != nil {
		if isTimeout(err) {
			return nil, BuildTimeout(req.Limits.BuildS, tail)
		}
		var ee *shell.ExitError
		if errors.As(err, &ee) {
			return nil, MapBuildError(tail)
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
		msg := fmt.Sprintf("closure is %s, limit is %s; largest paths:\n%s", humanBytes(size), humanBytes(req.Limits.ClosureBytes), largest)
		return nil, &Error{Code: "closure_too_large", Message: msg}
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
	return &Result{SystemClosure: outPath, ClosureBytes: size, Kernel: info.Kernel, Initrd: info.Initrd}, nil
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
	res, err := b.R.Run(ctx, "nix", "path-info", "-rS", path)
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

func humanBytes(n uint64) string {
	const gb = 1 << 30
	const mb = 1 << 20
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
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
