package isolation

import (
	"strconv"
	"strings"
	"testing"
)

// Row: guest cannot write the store. Mechanism: virtiofsd (unprivileged,
// namespace sandbox) serves a read-only bind of the store with `.links`
// masked by an empty tmpfs; the guest mounts the tag read-only.
func TestGuestCannotWriteStore(t *testing.T) {
	need(t, "EXEC_A")
	mustFail(t, inA(t, "touch /nix/store/repose-isolation-probe"), "touch /nix/store as dev")
	mustFail(t, inA(t, "sudo -n touch /nix/.ro-store/repose-isolation-probe"), "touch the virtio-fs share as root")
	mustFail(t, inA(t, "sudo -n sh -c 'echo x > /nix/.ro-store/.links/repose-isolation-probe'"), "write under .links")
	r := inA(t, "ls -A /nix/.ro-store/.links 2>&1 | head -c 400; echo; ls -A /nix/.ro-store/.links 2>/dev/null | wc -l")
	lines := strings.Split(strings.TrimSpace(r.out), "\n")
	count := strings.TrimSpace(lines[len(lines)-1])
	if n, err := strconv.Atoi(count); err == nil && n > 0 {
		t.Fatalf("the store's .links hard-link farm is visible from the guest (%d entries):\n%s", n, r)
	}
	t.Logf(".links from the guest: %s", strings.TrimSpace(r.out))
}

// Row: guest cannot see host block devices; only its own thin volume is a
// virtio-blk device.
func TestGuestSeesOneDisk(t *testing.T) {
	need(t, "EXEC_A")
	r := inA(t, "lsblk -dn -o NAME,TYPE | awk '$2==\"disk\"{print $1}'")
	mustSucceed(t, r, "lsblk in A")
	disks := strings.Fields(r.out)
	if len(disks) != 1 {
		t.Fatalf("guest sees %d disks, want 1: %v", len(disks), disks)
	}
	mustFail(t, inA(t, "test -e /dev/vg-guests"), "host volume group visible in A")
	t.Logf("guest disk: %s", disks[0])
}
