//go:build !linux

package snapshot

import "os"

// openDirect has no O_DIRECT off Linux; hostd runs only there.
func openDirect(string) *os.File { return nil }
