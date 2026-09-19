package sysdep

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Freezer freezes and thaws a filesystem. docs/workstreams/04-guestd.md: the
// FIFREEZE ioctl is used rather than the fsfreeze binary, because forking
// while the root filesystem is frozen is how a snapshot turns into a hang.
type Freezer interface {
	Freeze(path string) error
	Thaw(path string) error
}

// FIFREEZE and FITHAW are _IOWR('X', 119|120, int) from linux/fs.h.
// golang.org/x/sys/unix does not export them.
const (
	FIFREEZE uint = 0xC0045877
	FITHAW   uint = 0xC0045878
)

// IoctlFreezer is the real Freezer.
type IoctlFreezer struct{}

// Freeze blocks writes to the filesystem containing path.
func (IoctlFreezer) Freeze(path string) error { return freezeIoctl(path, FIFREEZE, "freeze") }

// Thaw releases it.
func (IoctlFreezer) Thaw(path string) error { return freezeIoctl(path, FITHAW, "thaw") }

func freezeIoctl(path string, req uint, what string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%s: open filesystem: %w", what, err)
	}
	defer f.Close() //nolint:errcheck // read-only handle; the ioctl result is what matters
	if err := unix.IoctlSetInt(int(f.Fd()), req, 0); err != nil {
		return fmt.Errorf("%s: ioctl: %w", what, err)
	}
	return nil
}
