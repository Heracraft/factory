package cli

import (
	"context"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

func TestResolveProjectOrder(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	client := newClient(fake.URL()+"/v1", staticToken("tok"))
	ctx := context.Background()

	p, err := client.CreateProject(ctx, CreateProjectRequest{Name: "todo-app", RemoteURL: "github.com/a/b", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := client.CreateProject(ctx, CreateProjectRequest{Name: "explicit-target", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("explicit project id wins over everything", func(t *testing.T) {
		cache := newProjectsCache()
		deps := resolveDeps{RemoteFor: func(string) string { return "github.com/a/b" }}
		res, err := resolveProject(ctx, client, t.TempDir(), "/cwd", other.ID, &cache, deps)
		if err != nil {
			t.Fatal(err)
		}
		if res.Project == nil || res.Project.ID != other.ID {
			t.Fatalf("got %+v, want %s", res.Project, other.ID)
		}
	})

	t.Run("explicit slug resolves via listing", func(t *testing.T) {
		cache := newProjectsCache()
		deps := resolveDeps{RemoteFor: func(string) string { return "" }}
		res, err := resolveProject(ctx, client, t.TempDir(), "/cwd", "explicit-target", &cache, deps)
		if err != nil {
			t.Fatal(err)
		}
		if res.Project == nil || res.Project.ID != other.ID {
			t.Fatalf("got %+v, want %s", res.Project, other.ID)
		}
	})

	t.Run("by_dir cache wins over remote", func(t *testing.T) {
		cache := newProjectsCache()
		cache.ByDir["/cwd"] = other.ID
		deps := resolveDeps{RemoteFor: func(string) string { return "github.com/a/b" }}
		res, err := resolveProject(ctx, client, t.TempDir(), "/cwd", "", &cache, deps)
		if err != nil {
			t.Fatal(err)
		}
		if res.Project == nil || res.Project.ID != other.ID {
			t.Fatalf("by_dir should have won: got %+v", res.Project)
		}
	})

	t.Run("remote resolves and populates the cache", func(t *testing.T) {
		cache := newProjectsCache()
		dir := t.TempDir()
		deps := resolveDeps{RemoteFor: func(string) string { return "github.com/a/b" }}
		res, err := resolveProject(ctx, client, dir, "/cwd", "", &cache, deps)
		if err != nil {
			t.Fatal(err)
		}
		if res.Project == nil || res.Project.ID != p.ID {
			t.Fatalf("got %+v, want %s", res.Project, p.ID)
		}
		if cache.ByRemote["github.com/a/b"].ProjectID != p.ID {
			t.Fatalf("cache not populated: %+v", cache.ByRemote)
		}
		reloaded, err := loadProjectsCache(dir)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.ByRemote["github.com/a/b"].ProjectID != p.ID {
			t.Fatalf("cache not persisted: %+v", reloaded.ByRemote)
		}
	})

	t.Run("no remote and nothing cached returns empty result", func(t *testing.T) {
		cache := newProjectsCache()
		deps := resolveDeps{RemoteFor: func(string) string { return "" }}
		res, err := resolveProject(ctx, client, t.TempDir(), "/cwd", "", &cache, deps)
		if err != nil {
			t.Fatal(err)
		}
		if res.Project != nil {
			t.Fatalf("expected no project, got %+v", res.Project)
		}
	})

	t.Run("unknown remote returns empty result but reports the remote", func(t *testing.T) {
		cache := newProjectsCache()
		deps := resolveDeps{RemoteFor: func(string) string { return "github.com/unknown/repo" }}
		res, err := resolveProject(ctx, client, t.TempDir(), "/cwd", "", &cache, deps)
		if err != nil {
			t.Fatal(err)
		}
		if res.Project != nil || res.Remote != "github.com/unknown/repo" {
			t.Fatalf("got %+v", res)
		}
	})
}

func TestErrNoProjectFoundMessages(t *testing.T) {
	e := errNoProjectFound("").(*exitError)
	if e.code != ExitProjectNotFound {
		t.Fatalf("code = %d", e.code)
	}
	e2 := errNoProjectFound("github.com/a/b").(*exitError)
	if e2.code != ExitProjectNotFound {
		t.Fatalf("code = %d", e2.code)
	}
}
