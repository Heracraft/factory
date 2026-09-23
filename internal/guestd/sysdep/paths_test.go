package sysdep

import "testing"

// I-209: a real guest's paths are absolute; systemd-run refuses a relative
// StandardOutput path, which is how every Switch failed.
func TestPathsAreAbsoluteOnARealGuest(t *testing.T) {
	var p Paths
	for got, want := range map[string]string{
		p.SwitchLog(): "/run/repose/switch.log",
		p.EtcEnv():    "/etc/repose/env",
		p.Proc():      "/proc",
		p.Home():      "/home/dev",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	if got := (Paths{Root: "/tmp/x"}).SwitchLog(); got != "/tmp/x/run/repose/switch.log" {
		t.Errorf("under a root: %q", got)
	}
}
