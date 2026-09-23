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
	if err := n.AddGuestRules(ctx, "g1", "10.64.4.2", "52:54:01:92:ab:cd", "tap-0192abcd"); err != nil {
		t.Fatal(err)
	}
	if err := n.Shape(ctx, "tap-0192abcd", 200); err != nil {
		t.Fatal(err)
	}
	if err := n.DelGuestRules(ctx, "g1", "10.64.4.2", "52:54:01:92:ab:cd", "tap-0192abcd"); err != nil {
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

// qdiscModel answers `tc qdisc` the way the kernel would for one tap, so
// Shape's migration and idempotence are tested against state, not a script.
type qdiscModel struct{ htb, ingress bool }

func (q *qdiscModel) handle(argv []string) (shell.Result, error) {
	fail := func() (shell.Result, error) {
		r := shell.Result{ExitCode: 2}
		return r, &shell.ExitError{Argv: argv, Result: r}
	}
	verb, last := argv[2], argv[len(argv)-1]
	switch verb + " " + last {
	case "show " + last:
		out := ""
		if q.htb {
			out += "qdisc htb 1: root refcnt 2 r2q 10 default 0x10 direct_packets_stat 0 direct_qlen 1000\n"
		} else {
			out += "qdisc fq_codel 0: root refcnt 2 limit 10240p flows 1024 quantum 1514\n"
		}
		if q.ingress {
			out += "qdisc ingress ffff: parent ffff:fff1 ----------------\n"
		}
		return shell.Result{Stdout: []byte(out)}, nil
	case "add ingress":
		if q.ingress {
			return fail()
		}
		q.ingress = true
	case "del ingress":
		if !q.ingress {
			return fail()
		}
		q.ingress = false
	case "del root":
		if !q.htb {
			return fail() // "Cannot delete qdisc with handle of zero."
		}
		q.htb = false
	}
	return shell.Result{}, nil
}

func TestShapeMigratesLegacyRootAndIsIdempotent(t *testing.T) {
	q := &qdiscModel{htb: true} // a tap shaped before I-217
	r := &shell.Fake{Scripts: []shell.Script{{Prefix: []string{"tc", "qdisc"}, Handle: q.handle}}}
	n := NewReal(r)
	ctx := context.Background()
	if err := n.Shape(ctx, "tap-0192abcd", 200); err != nil {
		t.Fatal(err)
	}
	if q.htb || !q.ingress {
		t.Fatalf("after the first Shape htb=%v ingress=%v; want the legacy root gone and the ingress policer in place", q.htb, q.ingress)
	}
	// a hostd restart re-applies, here with a new rate: nothing added twice
	if err := n.Shape(ctx, "tap-0192abcd", 100); err != nil {
		t.Fatal(err)
	}
	if err := n.Unshape(ctx, "tap-0192abcd"); err != nil {
		t.Fatal(err)
	}
	if q.htb || q.ingress {
		t.Fatalf("after Unshape htb=%v ingress=%v", q.htb, q.ingress)
	}
	if err := n.Unshape(ctx, "tap-0192abcd"); err != nil {
		t.Fatalf("a second Unshape must be a no-op: %v", err)
	}
	var got []string
	for _, c := range r.Calls[:len(r.Calls)-2] {
		got = append(got, strings.Join(c, " "))
	}
	golden, err := os.ReadFile("testdata/reshape.golden")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "\n")+"\n" != string(golden) {
		t.Fatalf("commands differ from testdata/reshape.golden:\n%s", strings.Join(got, "\n"))
	}
}
