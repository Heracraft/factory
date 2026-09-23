package cli

import (
	"fmt"
	"sync"
	"testing"
)

// Three `repose run`s started together in three directories each load
// projects.json, add their own project and save. Found live on 2026-09-23:
// only the last writer's entry survived, and the other two directories then
// said "no git remote" on the next plain `repose run`.
func TestProjectsCacheConcurrentSavesMerge(t *testing.T) {
	dir := t.TempDir()
	seed := newProjectsCache()
	seed.ByDir["/old"] = "p-old"
	seed.ByRemote["github.com/x/stale"] = CachedProject{ProjectID: "p-stale", Slug: "stale"}
	if err := saveProjectsCache(dir, seed); err != nil {
		t.Fatal(err)
	}

	caches := make([]ProjectsCache, 3)
	for i := range caches {
		c, err := loadProjectsCache(dir)
		if err != nil {
			t.Fatal(err)
		}
		caches[i] = c
	}
	// The first one also removes the stale remote, the way resolveProject
	// deletes a mapping that no longer resolves (I-152).
	delete(caches[0].ByRemote, "github.com/x/stale")

	var wg sync.WaitGroup
	for i := range caches {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rememberProject(&caches[i], fmt.Sprintf("github.com/x/r%d", i), fmt.Sprintf("/code/r%d", i),
				Project{ID: fmt.Sprintf("p%d", i), Slug: fmt.Sprintf("r%d", i)})
			if err := saveProjectsCache(dir, caches[i]); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()

	got, err := loadProjectsCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if got.ByDir[fmt.Sprintf("/code/r%d", i)] != fmt.Sprintf("p%d", i) {
			t.Errorf("dir mapping %d lost: %v", i, got.ByDir)
		}
		if got.ByRemote[fmt.Sprintf("github.com/x/r%d", i)].ProjectID != fmt.Sprintf("p%d", i) {
			t.Errorf("remote mapping %d lost", i)
		}
	}
	if got.ByDir["/old"] != "p-old" {
		t.Errorf("an entry nobody touched was lost: %v", got.ByDir)
	}
	if _, ok := got.ByRemote["github.com/x/stale"]; ok {
		t.Error("a removal by one process was undone by the others")
	}
}
