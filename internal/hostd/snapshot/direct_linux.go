package snapshot

import (
	"os"
	"syscall"
)

// openDirect opens dev for writing around the page cache, or returns nil
// where the filesystem refuses O_DIRECT (tmpfs, in tests).
func openDirect(dev string) *os.File {
	f, err := os.OpenFile(dev, os.O_WRONLY|syscall.O_DIRECT, 0)
	if err != nil {
		return nil
	}
	return f
}
