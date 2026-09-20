//go:build darwin

package cli

import (
	"bytes"
	"os/exec"
)

const keychainService = "repose"

func keychainAvailable() bool { return true }

func keychainGet(account string) (string, bool) {
	if account == "" {
		return "", false
	}
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", account, "-w").Output()
	if err != nil {
		return "", false
	}
	return string(bytes.TrimSpace(out)), true
}

func keychainSet(account, value string) error {
	_ = keychainDelete(account) // add-generic-password does not overwrite by default
	return exec.Command("security", "add-generic-password", "-s", keychainService, "-a", account, "-w", value, "-U").Run()
}

func keychainDelete(account string) error {
	return exec.Command("security", "delete-generic-password", "-s", keychainService, "-a", account).Run()
}
