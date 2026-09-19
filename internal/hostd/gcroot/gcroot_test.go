package gcroot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetGetRemovePrune(t *testing.T) {
	dir := t.TempDir()
	store := t.TempDir()
	_ = os.WriteFile(filepath.Join(store, "a"), nil, 0o644)
	r := Roots{Dir: filepath.Join(dir, "repose")}
	if err := r.Set("g1", filepath.Join(store, "a")); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.Exists("g1"); !ok {
		t.Fatal("root should exist")
	}
	if err := r.Set("g1", filepath.Join(store, "missing")); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.Exists("g1"); ok {
		t.Fatal("root to a missing target must report absent")
	}
	for _, rev := range []string{"p1-0001", "p1-0002", "p1-0003", "p1-0004", "p2-0001"} {
		if err := r.Set(RevisionRoot(rev), filepath.Join(store, "a")); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := r.PruneRevisions("p1", 3)
	if err != nil || len(removed) != 1 || removed[0] != "rev-p1-0001" {
		t.Fatalf("prune: %v %v", removed, err)
	}
	if err := r.Remove("g1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove("g1"); err != nil {
		t.Fatal("removing twice must be fine")
	}
	es, _ := r.List()
	if len(es) != 4 {
		t.Fatalf("expected 4 roots, got %v", es)
	}
}
