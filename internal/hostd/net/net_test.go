package net

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/shell"
)

func TestRealRendersDocumentedCommands(t *testing.T) {
	r := &shell.Fake{Scripts: []shell.Script{
		{Prefix: []string{"ip", "link", "show"}, Result: shell.Result{ExitCode: 1}},
		{Prefix: []string{"nft", "list", "counter"}, Result: shell.Result{ExitCode: 1}},
		{Prefix: []string{"nft", "-a", "list", "chain"}, Result: shell.Result{Stdout: []byte("table inet repose {\n\tchain guest_dyn {\n\t\tip saddr 10.64.4.2 counter name \"egress-g1\" # handle 17\n\t\tip saddr 10.64.4.3 counter name \"egress-g2\" # handle 18\n\t}\n}\n")}},
	}}
	n := NewReal(r)
	ctx := context.Background()
	if err := n.AddTap(ctx, "tap-0192abcd"); err != nil {
		t.Fatal(err)
	}
	if err := n.AddGuestRules(ctx, "g1", "10.64.4.2", "tap-0192abcd"); err != nil {
		t.Fatal(err)
	}
	if err := n.Shape(ctx, "tap-0192abcd", 200); err != nil {
		t.Fatal(err)
	}
	if err := n.DelGuestRules(ctx, "g1", "10.64.4.2", "tap-0192abcd"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range r.Calls {
		got = append(got, strings.Join(c, " "))
	}
	golden, err := os.ReadFile("testdata/create.golden")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n")+"\n" != string(golden) {
		t.Fatalf("commands differ from testdata/create.golden:\n%s", strings.Join(got, "\n"))
	}
}

func TestTapStatsAreGuestView(t *testing.T) {
	dir := t.TempDir()
	st := filepath.Join(dir, "tap-x", "statistics")
	_ = os.MkdirAll(st, 0o755)
	_ = os.WriteFile(filepath.Join(st, "tx_bytes"), []byte("100\n"), 0o644)
	_ = os.WriteFile(filepath.Join(st, "rx_bytes"), []byte("7\n"), 0o644)
	n := &Real{SysFS: dir}
	rx, tx, err := n.TapStats("tap-x")
	if err != nil || rx != 100 || tx != 7 {
		t.Fatalf("rx=%d tx=%d err=%v; guest rx must be the tap's tx", rx, tx, err)
	}
}
