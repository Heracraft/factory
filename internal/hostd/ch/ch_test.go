package ch

import (
	"os"
	"strings"
	"testing"
)

func TestArgsGolden(t *testing.T) {
	s := Spec{
		GuestDir: "/var/lib/repose/guests/0192f0a1-1111-7000-8000-000000000001",
		Kernel:   "/nix/store/abc-nixos-system/kernel", Initrd: "/nix/store/abc-nixos-system/initrd",
		Init: "/nix/store/abc-nixos-system/init", KernelParams: "loglevel=4 reboot=t panic=-1\n",
		IP: "10.64.4.2", Gateway: "10.64.4.1", Netmask: "255.255.252.0",
		Tap: "tap-0192f0a1", MAC: "52:54:01:92:f0:a1", CID: 1000,
		VolumeDev: "/dev/vg-guests/g-0192f0a1-1111-7000-8000-000000000001",
		VCPUs:     4, MemMiB: 8192, StoreTag: "ro-store",
	}
	got := strings.Join(s.Args(), "\n") + "\n"
	want, err := os.ReadFile("testdata/args.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("argv differs from testdata/args.golden:\n%s", got)
	}
}
