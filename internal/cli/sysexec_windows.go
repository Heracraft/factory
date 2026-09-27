//go:build windows

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"syscall"
)

// sysExec has no process-image-replace primitive on Windows (GoReleaser
// does not target it, DESIGN.md §10's four platforms are darwin/linux),
// so it runs the child and exits with its code instead.
func sysExec(name string, args []string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}

// spawnDetached is never called on Windows (no multiplexing, so no
// session helper).
func spawnDetached(name string, args []string, extraEnv ...string) error { return nil }

// startDetached starts cmd outside the console's process group, with
// stdin and stdout on NUL and stderr captured, so it outlives the CLI:
// the background forward of `repose browser`. The caller waits on cmd.
func startDetached(cmd *exec.Cmd) (*bytes.Buffer, error) {
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = devnull.Close() }()
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, &stderr
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | detachedProcess, HideWindow: true}
	return &stderr, cmd.Start()
}
