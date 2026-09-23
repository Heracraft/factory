package lvm

import (
	"context"
	"strings"
	"testing"

	"github.com/heracraft/repose/internal/hostd/shell"
)

func TestRealRendersDocumentedCommands(t *testing.T) {
	r := &shell.Fake{Scripts: []shell.Script{
		{Prefix: []string{"lvs", "vg-guests/g-1"}, Result: shell.Result{ExitCode: 5}},
		{Prefix: []string{"lvs", "vg-guests/snap-1"}, Result: shell.Result{ExitCode: 5}},
		{Prefix: []string{"blkid"}, Result: shell.Result{ExitCode: 2}},
		{Prefix: []string{"lvs", "--noheadings"}, Result: shell.Result{Stdout: []byte("  42949672960|12.50\n")}},
	}}
	l := &Real{VG: "vg-guests", Pool: "thin", R: r}
	ctx := context.Background()
	if err := l.CreateVolume(ctx, "g-1", 42949672960); err != nil {
		t.Fatal(err)
	}
	if err := l.Mkfs(ctx, "g-1"); err != nil {
		t.Fatal(err)
	}
	if err := l.Snapshot(ctx, "g-1", "snap-1"); err != nil {
		t.Fatal(err)
	}
	if err := l.ExtendVolume(ctx, "g-1", 85899345920); err != nil {
		t.Fatal(err)
	}
	size, used, err := l.VolumeStats(ctx, "g-1")
	if err != nil || size != 42949672960 || used != 5368709120 {
		t.Fatalf("stats: %d %d %v", size, used, err)
	}
	want := []string{
		"lvcreate -V 42949672960b -T vg-guests/thin -n g-1",
		"mkfs.ext4 -q -L guest -E lazy_itable_init=1 /dev/vg-guests/g-1",
		"lvcreate -s -n snap-1 vg-guests/g-1",
		"lvextend -L 85899345920b vg-guests/g-1",
	}
	var got []string
	for _, c := range r.Calls {
		s := strings.Join(c, " ")
		for _, w := range want {
			if s == w {
				got = append(got, s)
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("expected commands %v, matched %v in %v", want, got, r.Calls)
	}
}

func TestFakeLifecycle(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	_ = f.CreateVolume(ctx, "g-1", 10)
	_ = f.Mkfs(ctx, "g-1")
	f.SetData("g-1", []byte("hello"))
	if err := f.Snapshot(ctx, "g-1", "s"); err != nil {
		t.Fatal(err)
	}
	if string(f.GetData("s")) != "hello" {
		t.Fatal("snapshot did not copy data")
	}
	if err := f.RemoveVolume(ctx, "s"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := f.VolumeExists(ctx, "s"); ok {
		t.Fatal("snapshot still exists")
	}
}
