package cli

import (
	"context"
)

// resolveDeps lets tests replace the git and filesystem calls resolution
// makes.
type resolveDeps struct {
	RemoteFor func(cwd string) string // "" means no remote
}

func defaultResolveDeps() resolveDeps {
	return resolveDeps{RemoteFor: gitRemoteOrigin}
}

// ResolveResult is what resolveProject found.
type ResolveResult struct {
	Project *Project // nil if nothing matched
	Remote  string   // normalised remote for cwd, "" if none
}

// resolveProject implements docs/workstreams/07-cli.md §5.3's order. It
// mutates and saves cache in place when a lookup fills in something the
// cache did not have. explicit is --project or $REPOSE_PROJECT; empty
// when neither was given.
func resolveProject(ctx context.Context, client *Client, dir, cwd, explicit string, cache *ProjectsCache, deps resolveDeps) (*ResolveResult, error) {
	if explicit != "" {
		p, err := findByIDOrSlug(ctx, client, explicit)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, exitf(ExitProjectNotFound, "No repose project matches --project %s.", explicit)
		}
		rememberProject(cache, "", cwd, *p)
		_ = saveProjectsCache(dir, *cache)
		return &ResolveResult{Project: p}, nil
	}

	if id, ok := cache.ByDir[cwd]; ok {
		if p, err := client.GetProject(ctx, id); err == nil {
			return &ResolveResult{Project: p}, nil
		}
		// Stale cache entry (project destroyed, or moved accounts):
		// fall through to remote-based resolution rather than erroring.
	}

	remote := deps.RemoteFor(cwd)
	if remote == "" {
		return &ResolveResult{}, nil
	}
	if cached, ok := cache.ByRemote[remote]; ok {
		if p, err := client.GetProject(ctx, cached.ProjectID); err == nil {
			return &ResolveResult{Project: p, Remote: remote}, nil
		}
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.RemoteURL == remote {
			rememberProject(cache, remote, cwd, p)
			_ = saveProjectsCache(dir, *cache)
			return &ResolveResult{Project: &p, Remote: remote}, nil
		}
	}
	return &ResolveResult{Remote: remote}, nil
}

func findByIDOrSlug(ctx context.Context, client *Client, idOrSlug string) (*Project, error) {
	if p, err := client.GetProject(ctx, idOrSlug); err == nil {
		return p, nil
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Slug == idOrSlug {
			return &p, nil
		}
	}
	return nil, nil
}

func rememberProject(cache *ProjectsCache, remote, cwd string, p Project) {
	if remote != "" {
		cache.ByRemote[remote] = CachedProject{ProjectID: p.ID, Slug: p.Slug, Name: p.Name}
	}
	if cwd != "" {
		cache.ByDir[cwd] = p.ID
	}
}

// errNoProjectFound is what non-run commands report when resolution finds
// nothing (07-cli.md §5.3 step 4).
func errNoProjectFound(remote string) error {
	if remote == "" {
		return exitf(ExitProjectNotFound, "No repose project here, and this directory has no git remote. Pass --project.")
	}
	return exitf(ExitProjectNotFound, "No repose project for %s. Run `repose run` here to create one, or pass --project.", remote)
}

// errNoRemoteNoName is `run`'s error when there is no git remote and no
// --name (07-cli.md §5.3).
func errNoRemoteNoName() error {
	return exitf(ExitUsage, "This directory has no git remote. Pass --name NAME to create a project anyway.")
}
