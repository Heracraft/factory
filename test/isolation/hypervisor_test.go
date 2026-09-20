package isolation

import (
	"strings"
	"testing"
)

// Row: a guest escape does not land as root (review H-2, DECISIONS I-51).
// The hypervisor process of guest A runs as the hostd user inside the
// sandboxed transient unit, and the unit says so.
func TestHypervisorRunsAsHostdUser(t *testing.T) {
	need(t, "HOST_EXEC", "A_GUEST_ID")
	unit := "guest@" + env("A_GUEST_ID")
	r := onHost(t, "ps -o user= -p $(systemctl show -p MainPID --value "+unit+")")
	mustSucceed(t, r, "hypervisor process user")
	if got := strings.TrimSpace(r.out); got != "hostd" {
		t.Fatalf("hypervisor runs as %q, want hostd", got)
	}
	r = onHost(t, "systemctl show "+unit+" -p User,NoNewPrivileges,DevicePolicy,ProtectSystem,CapabilityBoundingSet,RestrictAddressFamilies")
	mustSucceed(t, r, "unit properties")
	t.Logf("unit:\n%s", strings.TrimSpace(r.out))
	for _, want := range []string{"User=hostd", "NoNewPrivileges=yes", "DevicePolicy=closed", "ProtectSystem=strict"} {
		if !strings.Contains(r.out, want) {
			t.Fatalf("unit lacks %s", want)
		}
	}
	// The volume node the unit was allowed is group hostd through the udev rule.
	r = onHost(t, "stat -L -c %U:%G:%a /dev/vg-guests/g-"+env("A_GUEST_ID"))
	mustSucceed(t, r, "volume node")
	if got := strings.TrimSpace(r.out); got != "root:hostd:660" {
		t.Fatalf("volume node is %s, want root:hostd:660", got)
	}
}
