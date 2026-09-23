//go:build windows

package cli

import (
	"os"
	"os/exec"
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
