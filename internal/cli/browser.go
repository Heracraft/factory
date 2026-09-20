package cli

import (
	"os/exec"
	"runtime"
)

// openBrowser opens url in the default browser. Failure is not fatal
// anywhere it is called (07-cli.md §6: "Browser cannot open: print the URL
// and continue").
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// browserAvailable is DESIGN.md §8's "if a browser is available": not
// forced off, and either a display is set or the OS always has one.
func browserAvailable(noBrowser bool, guestEnv bool, display string, goos string) bool {
	if noBrowser || guestEnv {
		return false
	}
	if goos == "darwin" || goos == "windows" {
		return true
	}
	return display != ""
}
