package cli

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points HOME (and XDG_CONFIG_HOME) at a throwaway directory for
// the whole package, so no test can write the developer's real
// ~/.ssh/repose or ~/.config/repose. A restore test that renewed the
// certificate (I-188) overwrote the dev box's real certificate, config and
// known_hosts with the fake CA's before this existed. Tests that need
// their own home still call withHome.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "repose-cli-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", home)
	_ = os.Setenv("XDG_CONFIG_HOME", home+"/.config")
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
