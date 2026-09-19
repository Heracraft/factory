// Package virtiofs runs one virtiofsd per guest as a transient unit,
// exporting the host's store export read-only. virtiofsd has no read-only
// flag; read-only holds because the virtiofsd user cannot write under the
// export and the guest mounts the tag ro.
package virtiofs

import (
	"context"

	"github.com/heracraft/repose/internal/hostd/systemd"
)

// Config is where the export lives and who serves it.
type Config struct {
	SharedDir string // /run/repose/store-export
	User      string // virtiofsd
	Group     string // virtiofsd
	Binary    string // virtiofsd
}

// Unit is the transient unit name for a guest.
func Unit(guestID string) string { return "virtiofsd@" + guestID }

// Start launches virtiofsd for a guest; running already is not an error.
func Start(ctx context.Context, sd systemd.Systemd, cfg Config, guestID, socket string) error {
	bin := cfg.Binary
	if bin == "" {
		bin = "virtiofsd"
	}
	props := []string{"User=" + cfg.User, "Group=" + cfg.Group, "MemoryMax=1G", "Slice=guests.slice"}
	argv := []string{bin, "--socket-path", socket, "--shared-dir", cfg.SharedDir, "--sandbox", "chroot", "--cache", "auto", "--xattr"}
	return sd.Run(ctx, Unit(guestID), props, argv)
}

// Stop ends a guest's virtiofsd.
func Stop(ctx context.Context, sd systemd.Systemd, guestID string) error {
	return sd.Stop(ctx, Unit(guestID))
}
