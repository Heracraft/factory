package systemd

import (
	"context"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/shell"
)

func TestRunRendersSystemdRun(t *testing.T) {
	r := &shell.Fake{Scripts: []shell.Script{
		{Prefix: []string{"systemctl", "is-active"}, Result: shell.Result{Stdout: []byte("inactive\n"), ExitCode: 3}},
		{Prefix: []string{"systemctl", "list-units"}, Result: shell.Result{Stdout: []byte("guest@a.service loaded active running a\nguest@b.service loaded active running b\n")}},
	}}
	s := NewReal(r)
	err := s.Run(context.Background(), "guest@g1", []string{"MemoryMax=8704M", "CPUQuota=400%"}, []string{"cloud-hypervisor", "--api-socket", "/x"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(r.Calls[1], " ")
	want := "systemd-run --unit guest@g1 --collect --property KillMode=mixed --property MemoryMax=8704M --property CPUQuota=400% -- cloud-hypervisor --api-socket /x"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	units, err := s.ListUnits(context.Background(), "guest@*")
	if err != nil || len(units) != 2 || units[0] != "guest@a.service" {
		t.Fatalf("units %v %v", units, err)
	}
}
