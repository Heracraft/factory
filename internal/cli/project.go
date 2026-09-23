package cli

import (
	"context"
	"errors"
)

// resolveDeps lets tests replace the git and filesystem calls resolution
// makes.
type resolveDeps struct {
	RemoteFor func(cwd string) string // "" means no remote
	RootFor   func(cwd string) string // the repo root, "" outside a repo
}

func defaultResolveDeps() resolveDeps {
	return resolveDeps{RemoteFor: gitRemoteOrigin, RootFor: gitRepoRoot}
}

// ResolveResult is what resolveProject found.
type ResolveResult struct {
	Project *Project // nil if nothing matched
	Remote  string   // normalised remote for cwd, "" if none
}

// dirKey is the by_dir key for cwd: the repository root when cwd is inside
// one, so `repose run` from a subdirectory finds the same project, else
// cwd itself.
func dirKey(cwd string, deps resolveDeps) string {
	if deps.RootFor != nil {
		if root := deps.RootFor(cwd); root != "" {
			return root
		}
	}
	return cwd
}

// resolveProject implements docs/workstreams/07-cli.md §5.3's order. It
// mutates and saves cache in place when a lookup fills in something the
// cache did not have. explicit is the positional PROJECT, --project or
// $REPOSE_PROJECT; empty when none was given.
//
// Two rules keep the directory cache honest (DECISIONS I-152): an
// explicit project is never written to by_dir (naming a project is not a
// statement about the directory you happen to be in), and a by_dir entry
// is only believed when that project's remote is the directory's remote
// (or neither has one), so a stale or poisoned entry can never send
// `repose run` in one repository to another repository's guest.
func resolveProject(ctx context.Context, client *Client, dir, cwd, explicit string, cache *ProjectsCache, deps resolveDeps) (*ResolveResult, error) {
	if explicit != "" {
		p, err := findByIDOrSlug(ctx, client, explicit)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, exitf(ExitProjectNotFound, "No repose project is called %s. `repose projects` lists yours.", explicit)
		}
		return &ResolveResult{Project: p}, nil
	}

	remote := deps.RemoteFor(cwd)
	key := dirKey(cwd, deps)
	for _, k := range uniqueStrings(key, cwd) {
		id, ok := cache.ByDir[k]
		if !ok {
			continue
		}
		p, err := client.GetProject(ctx, id)
		if err == nil && p.RemoteURL == remote {
			return &ResolveResult{Project: p, Remote: remote}, nil
		}
		if err != nil && !isNotFound(err) {
			return nil, err
		}
		// Destroyed, moved, or a different repository's project (the
		// v0.1.4 cache wrote by_dir for every --project): forget it.
		delete(cache.ByDir, k)
		_ = saveProjectsCache(dir, *cache)
	}

	if remote == "" {
		return &ResolveResult{}, nil
	}
	if cached, ok := cache.ByRemote[remote]; ok {
		p, err := client.GetProject(ctx, cached.ProjectID)
		if err == nil && p.RemoteURL == remote {
			return &ResolveResult{Project: p, Remote: remote}, nil
		}
		if err != nil && !isNotFound(err) {
			return nil, err
		}
		delete(cache.ByRemote, remote)
		_ = saveProjectsCache(dir, *cache)
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.RemoteURL == remote {
			rememberProject(cache, remote, "", p)
			_ = saveProjectsCache(dir, *cache)
			return &ResolveResult{Project: &p, Remote: remote}, nil
		}
	}
	return &ResolveResult{Remote: remote}, nil
}

func uniqueStrings(a, b string) []string {
	if a == b {
		return []string{a}
	}
	return []string{a, b}
}

func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Code == "not_found" || apiErr.Status == 404)
}

func findByIDOrSlug(ctx context.Context, client *Client, idOrSlug string) (*Project, error) {
	if looksLikeUUID(idOrSlug) {
		if p, err := client.GetProject(ctx, idOrSlug); err == nil {
			return p, nil
		} else if !isNotFound(err) && !isInvalid(err) {
			return nil, err
		}
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Slug == idOrSlug || p.ID == idOrSlug {
			return &p, nil
		}
	}
	return nil, nil
}

func isInvalid(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Code == "invalid" || apiErr.Status == 400)
}

// rememberProject caches a project under its remote and, when dir is not
// "", under that directory (only `run` creating a project writes a
// directory: a --name project with no remote has nothing else to be found
// by).
func rememberProject(cache *ProjectsCache, remote, dir string, p Project) {
	if remote != "" {
		cache.ByRemote[remote] = CachedProject{ProjectID: p.ID, Slug: p.Slug, Name: p.Name}
	}
	if dir != "" {
		cache.ByDir[dir] = p.ID
	}
}

// errNoProjectFound is what non-run commands report when resolution finds
// nothing (07-cli.md §5.3 step 4).
func errNoProjectFound(remote string) error { return errNoProjectFoundFor(remote, "") }

// errNoProjectFoundFor names the command the user typed ("repose attach")
// in the hint when it is known.
func errNoProjectFoundFor(remote, command string) error {
	usage := "`repose <command> PROJECT`"
	if command != "" {
		usage = "`" + command + " PROJECT`"
	}
	if remote == "" {
		return exitf(ExitProjectNotFound, "No repose project here, and this directory has no git remote. Name one: %s (`repose projects` lists them).", usage)
	}
	return exitf(ExitProjectNotFound, "No repose project for %s. Run `repose run` here to create one, or name one: %s.", remote, usage)
}

// errNoRemoteNoName is `run`'s error when there is no git remote and no
// --name (07-cli.md §5.3).
func errNoRemoteNoName() error {
	return exitf(ExitUsage, "This directory has no git remote. Pass --name NAME to create a project anyway.")
}
