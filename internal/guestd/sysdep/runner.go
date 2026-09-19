package sysdep

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
)

// RunSpec is one command guestd runs outside itself.
type RunSpec struct {
	Argv []string
	// User is "" or "root" for root, "dev" for the guest user. docs/
	// workstreams/04-guestd.md: Exec drops privileges with setpriv.
	User string
	// Dir is the working directory; empty means guestd's own.
	Dir string
	// Env replaces the environment entirely when non-nil.
	Env []string
	// Stdin is fed to the command.
	Stdin []byte
	// MaxOutput caps each of stdout and stderr. Zero means DefaultMaxOutput.
	MaxOutput int
}

// DefaultMaxOutput is the per-stream cap when a RunSpec does not set one.
const DefaultMaxOutput = 64 << 10

// RunResult is what a command produced. Stdout and Stderr are the tail of each
// stream, capped at MaxOutput.
type RunResult struct {
	ExitCode  int
	Stdout    []byte
	Stderr    []byte
	Truncated bool
}

// Runner runs a command. The real implementation forks; the fake records.
type Runner interface {
	Run(ctx context.Context, spec RunSpec) (RunResult, error)
}

// ExecRunner is the real Runner.
type ExecRunner struct {
	// SetprivPath overrides the setpriv binary; empty means look it up on PATH.
	SetprivPath string
}

// DevUID and DevGID are the guest user's ids
// (docs/interfaces/guest-conventions.md: dev is uid 1000).
const (
	DevUID = 1000
	DevGID = 100
)

// Run executes spec. A non-zero exit is not an error: it comes back in
// RunResult.ExitCode, because every caller reports it rather than failing.
func (r ExecRunner) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	if len(spec.Argv) == 0 {
		return RunResult{}, errors.New("run: empty argv")
	}
	argv := spec.Argv
	if spec.User == "dev" {
		var err error
		argv, err = r.setprivArgv(argv)
		if err != nil {
			return RunResult{}, err
		}
	}

	max := spec.MaxOutput
	if max <= 0 {
		max = DefaultMaxOutput
	}
	var stdout, stderr tailBuffer
	stdout.max, stderr.max = max, max

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = spec.Env
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(spec.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(spec.Stdin)
	}
	// Its own process group, so the context's kill reaches children too.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	err := cmd.Run()
	res := RunResult{
		Stdout:    stdout.Bytes(),
		Stderr:    stderr.Bytes(),
		Truncated: stdout.truncated || stderr.truncated,
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.ExitCode = 0
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	default:
		return res, fmt.Errorf("run %s: %w", argv[0], err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return res, fmt.Errorf("run %s: %w", argv[0], ctxErr)
	}
	return res, nil
}

// setprivArgv wraps argv so it runs as dev. docs/workstreams/04-guestd.md
// names setpriv; it is in util-linux, which every NixOS guest has.
func (r ExecRunner) setprivArgv(argv []string) ([]string, error) {
	path := r.SetprivPath
	if path == "" {
		var err error
		path, err = exec.LookPath("setpriv")
		if err != nil {
			return nil, fmt.Errorf("setpriv not found, cannot drop privileges: %w", err)
		}
	}
	uid, gid := DevUID, DevGID
	if u, err := user.Lookup("dev"); err == nil {
		if n, err := strconv.Atoi(u.Uid); err == nil {
			uid = n
		}
		if n, err := strconv.Atoi(u.Gid); err == nil {
			gid = n
		}
	}
	wrapped := []string{
		path,
		"--reuid=" + strconv.Itoa(uid),
		"--regid=" + strconv.Itoa(gid),
		"--init-groups",
		"--",
	}
	return append(wrapped, argv...), nil
}

// DevEnv is the environment a command run as dev gets: enough to find binaries
// and the home directory, and nothing carried over from guestd's own.
func DevEnv(p Paths, user string) []string {
	home, name := p.Home(), "dev"
	if user == "" || user == "root" {
		home, name = "/root", "root"
	}
	env := []string{
		"PATH=/run/current-system/sw/bin:/usr/bin:/bin",
		"HOME=" + home,
		"USER=" + name,
		"LOGNAME=" + name,
		"SHELL=/bin/sh",
		"TERM=dumb",
	}
	for _, k := range []string{"TZ", "LANG", "REPOSE_PROJECT", "REPOSE"} {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// tailBuffer keeps at most max bytes, discarding from the front, so a command
// that floods stdout cannot exhaust the guest's memory and the interesting end
// of the output survives.
type tailBuffer struct {
	buf       []byte
	max       int
	truncated bool
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if t.max <= 0 {
		return n, nil
	}
	if len(p) > t.max {
		t.truncated = true
		p = p[len(p)-t.max:]
	}
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.truncated = true
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return n, nil
}

func (t *tailBuffer) Bytes() []byte { return t.buf }
