//go:build !windows

package cli

import (
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
