// Package virtiofs runs one virtiofsd per guest as a transient unit,
// exporting the host's store export read-only. virtiofsd has no read-only
// flag; read-only holds because the virtiofsd user cannot write under the
// export (a read-only bind mount) and the guest mounts the tag ro.
//
// The sandbox is `namespace`, not `chroot`: chroot(2) needs CAP_SYS_CHROOT,
// and virtiofsd 1.14 refuses it outright for a non-root user ("sandbox mode
// 'chroot' can only be used by root"). Namespace mode unshares a user and
// mount namespace and pivot_roots into the export, which is what an
// unprivileged process can do (DECISIONS I-48; the host enables user
// namespaces in nix/hosts/kernel.nix for exactly this).
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
	// SocketGroup is chgrp'd onto the vhost-user socket (mode 0660) so the
	// hypervisor's user, not virtiofsd's, can connect to it.
	SocketGroup string // hostd
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
	// --no-announce-submounts: the export masks .links with a tmpfs mount
	// (host-conventions.md). Announced, the guest sees it as a separate
	// virtiofs mount inside the overlay's lower layer, and overlayfs answers
	// every lookup crossing into it with EREMOTE ("Object is remote"), which
	// killed the guest's nix-daemon at its first mkdir of /nix/store/.links
	// (DECISIONS I-65). Flattened, it is an empty directory like any other.
	argv := []string{bin, "--socket-path", socket, "--shared-dir", cfg.SharedDir, "--sandbox", "namespace", "--cache", "auto", "--xattr", "--no-announce-submounts"}
	if cfg.SocketGroup != "" {
		argv = append(argv, "--socket-group", cfg.SocketGroup)
	}
	return sd.Run(ctx, Unit(guestID), props, argv)
}

// Stop ends a guest's virtiofsd.
func Stop(ctx context.Context, sd systemd.Systemd, guestID string) error {
	return sd.Stop(ctx, Unit(guestID))
}
