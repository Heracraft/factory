//go:build !windows

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"syscall"
)

// sysExec replaces the current process image with name+args (07-cli.md
// §7 checklist: "ps shows no repose parent during a session").
func sysExec(name string, args []string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	argv := append([]string{name}, args...)
	return syscall.Exec(path, argv, os.Environ())
}

// spawnDetached starts name+args in its own process group with stdio on
// /dev/null and does not wait for it: the session helper, which must
// outlive the exec into ssh and never see the terminal's signals.
func spawnDetached(name string, args []string, extraEnv ...string) error {
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = devnull.Close() }()
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// startDetached starts cmd in its own session with stdin and stdout on
// /dev/null and stderr captured, so it outlives the CLI and never sees
// the terminal's signals: the background forward of `repose browser`. The
// caller waits on cmd itself.
func startDetached(cmd *exec.Cmd) (*bytes.Buffer, error) {
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = devnull.Close() }()
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return &stderr, cmd.Start()
}
