// Package system implements the Switch request of
// docs/interfaces/vsock-guestd.md: activate a system closure the host built
// and put in the shared store, without rebooting unless the kernel or initrd
// changed.
package system

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

// OutputCap is the cap on the output returned to hostd
// (docs/interfaces/vsock-guestd.md: 32 KB).
const OutputCap = 32 << 10

// DefaultTimeout bounds a switch (docs/workstreams/04-guestd.md: 10 minutes).
const DefaultTimeout = 10 * time.Minute

// Handler runs switches.
type Handler struct {
	paths   sysdep.Paths
	run     sysdep.Runner
	timeout time.Duration
	log     *slog.Logger
	// reboot is the command used to reboot after a forced boot-activation.
	// Named so the unit test can observe it without rebooting the test host.
	rebootArgv []string
}

// New builds the handler.
func New(p sysdep.Paths, run sysdep.Runner, timeout time.Duration, log *slog.Logger) *Handler {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Handler{
		paths:      p,
		run:        run,
		timeout:    timeout,
		log:        log,
		rebootArgv: []string{"systemctl", "reboot"},
	}
}

// Switch activates closure. It returns needs_reboot without doing anything
// when the kernel or initrd differ and force is false, because rebooting a
// guest under the user's nose loses the agent that was running in it.
func (h *Handler) Switch(ctx context.Context, closure string, force bool, registration []byte) (*guestdv1.SwitchResult, error) {
	real, err := h.resolveClosure(closure)
	if err != nil {
		return nil, err
	}
	if len(registration) > 0 {
		if err := h.RegisterPaths(ctx, registration); err != nil {
			return nil, err
		}
	}

	changed, err := h.kernelChanged(real)
	if err != nil {
		return nil, err
	}
	if changed && !force {
		h.log.Info("switch needs a reboot; nothing applied",
			"event", "switch", "result", "needs_reboot")
		return &guestdv1.SwitchResult{NeedsReboot: true}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	// The profile is set first so that a reboot, whether this switch asks for
	// one or the guest is restarted later, lands on the new system.
	if err := h.setProfile(ctx, closure); err != nil {
		return nil, err
	}

	action := "switch"
	if changed {
		action = "boot"
	}
	res, runErr := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{filepath.Join(real, "bin", "switch-to-configuration"), action},
		MaxOutput: OutputCap,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	output := combine(res.Stdout, res.Stderr, OutputCap)
	if runErr != nil {
		h.log.Error("switch-to-configuration did not run",
			"event", "switch", "result", "error", "action", action)
		return &guestdv1.SwitchResult{Output: output},
			sysdep.Errf(sysdep.CodeInternal, "switch-to-configuration %s: %w", action, runErr)
	}
	if res.ExitCode != 0 {
		// The old system stays active; hostd surfaces this as an ApplyConfig
		// failure with the output, which is why output is returned either way.
		h.log.Warn("switch-to-configuration failed; previous system still active",
			"event", "switch", "result", "failed", "exit_code", res.ExitCode, "action", action)
		return &guestdv1.SwitchResult{Output: output},
			sysdep.Errf(sysdep.CodeInternal, "switch-to-configuration %s exited %d", action, res.ExitCode)
	}

	if !changed {
		h.log.Info("system switched", "event", "switch", "result", "ok")
		return &guestdv1.SwitchResult{Output: output}, nil
	}

	h.log.Info("system activated for boot; rebooting",
		"event", "switch", "result", "rebooting")
	if _, err := h.run.Run(ctx, sysdep.RunSpec{Argv: h.rebootArgv, MaxOutput: 4 << 10}); err != nil {
		return &guestdv1.SwitchResult{Output: output},
			sysdep.Errf(sysdep.CodeInternal, "reboot after boot activation: %w", err)
	}
	return &guestdv1.SwitchResult{Rebooted: true, Output: output}, nil
}

// resolveClosure checks that the closure is a store path and that the store
// actually holds it. A missing path means the host garbage-collected it, which
// is the store_path_missing case.
func (h *Handler) resolveClosure(closure string) (string, error) {
	if closure == "" {
		return "", sysdep.Invalid("switch: system_closure is empty")
	}
	if !strings.HasPrefix(closure, "/nix/store/") || strings.Contains(closure, "..") {
		return "", sysdep.Invalid("switch: system_closure is not a /nix/store path")
	}
	real := filepath.Join(h.paths.Root, closure)
	if _, err := os.Stat(real); err != nil {
		if os.IsNotExist(err) {
			return "", sysdep.NotFound("switch: %s is not in the store share", closure)
		}
		return "", sysdep.Errf(sysdep.CodeInternal, "switch: stat closure: %w", err)
	}
	return real, nil
}

// kernelChanged compares the closure's kernel and initrd with the running
// system's. Either differing means a reboot.
func (h *Handler) kernelChanged(real string) (bool, error) {
	cur := h.paths.CurrentSystem()
	for _, name := range []string{"kernel", "initrd"} {
		want, err := resolve(filepath.Join(real, name))
		if err != nil {
			return false, sysdep.Errf(sysdep.CodeInternal, "switch: read new %s: %w", name, err)
		}
		have, err := resolve(filepath.Join(cur, name))
		if err != nil {
			// No running system to compare with (first activation): treat it
			// as unchanged so the switch proceeds rather than demanding a
			// reboot nobody can grant.
			h.log.Debug("no running system to compare against", "event", "switch", "part", name)
			continue
		}
		if want != have {
			return true, nil
		}
	}
	return false, nil
}

// resolve follows a symlink if there is one, and otherwise returns the path,
// so a closure that stores the kernel as a plain file compares too.
func resolve(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(target) {
		return target, nil
	}
	return filepath.Join(filepath.Dir(path), target), nil
}

// RegisterPaths loads a `nix-store --dump-db` listing into the guest's nix
// database and leaves the stamp guest units wait for. Paths that arrive
// through the shared store are on disk but unknown to the database until
// this runs; `nix-env --set`, home-manager's activation and any user
// `nix` command that touches them fail without it (DECISIONS I-67).
// Loading is idempotent: a listing that is already registered changes
// nothing.
func (h *Handler) RegisterPaths(ctx context.Context, registration []byte) error {
	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"nix-store", "--load-db"},
		Stdin:     registration,
		MaxOutput: 8 << 10,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	if err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "register paths: %w", err)
	}
	if res.ExitCode != 0 {
		return sysdep.Errf(sysdep.CodeInternal, "register paths: nix-store --load-db exited %d", res.ExitCode)
	}
	if err := os.WriteFile(h.paths.PathsRegistered(), []byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "register paths: stamp: %w", err)
	}
	h.log.Info("store paths registered", "event", "register_paths", "bytes", len(registration))
	return nil
}

// setProfile points /nix/var/nix/profiles/system at the new closure.
func (h *Handler) setProfile(ctx context.Context, closure string) error {
	res, err := h.run.Run(ctx, sysdep.RunSpec{
		Argv:      []string{"nix-env", "--profile", h.paths.SystemProfile(), "--set", closure},
		MaxOutput: 8 << 10,
		Env:       sysdep.DevEnv(h.paths, "root"),
	})
	if err != nil {
		return sysdep.Errf(sysdep.CodeInternal, "set system profile: %w", err)
	}
	if res.ExitCode != 0 {
		return sysdep.Errf(sysdep.CodeInternal, "set system profile: nix-env exited %d", res.ExitCode)
	}
	return nil
}

// combine appends stderr to stdout and keeps the tail within cap, because the
// end of a failed activation is the part that says why.
func combine(stdout, stderr []byte, limit int) []byte {
	out := make([]byte, 0, len(stdout)+len(stderr)+1)
	out = append(out, stdout...)
	if len(stderr) > 0 {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, '\n')
		}
		out = append(out, stderr...)
	}
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
