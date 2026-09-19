// Package gcroot manages /nix/var/nix/gcroots/repose/: one symlink per
// guest (its system closure) and per kept build revision. A path with a
// root here survives nix-collect-garbage.
package gcroot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Roots is the directory of roots.
type Roots struct {
	Dir string
}

// Entry is one root.
type Entry struct {
	Name   string
	Target string
}

// Set points name at target atomically (ln -sfn semantics).
func (r Roots) Set(name, target string) error {
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return fmt.Errorf("gcroot: %w", err)
	}
	tmp := filepath.Join(r.Dir, "."+name+".tmp")
	_ = os.Remove(tmp) // leftover from an interrupted Set; Symlink reports a real problem
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("gcroot %s: %w", name, err)
	}
	if err := os.Rename(tmp, filepath.Join(r.Dir, name)); err != nil {
		_ = os.Remove(tmp) // rename failed; the temp link is not a root anyone needs
		return fmt.Errorf("gcroot %s: %w", name, err)
	}
	return nil
}

// Get returns the target of a root, or ErrNotExist.
func (r Roots) Get(name string) (string, error) {
	return os.Readlink(filepath.Join(r.Dir, name))
}

// Exists reports whether the root exists and its target does too.
func (r Roots) Exists(name string) (bool, error) {
	t, err := r.Get(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(t); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Remove deletes a root; a missing root is not an error.
func (r Roots) Remove(name string) error {
	err := os.Remove(filepath.Join(r.Dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// List returns every root sorted by name.
func (r Roots) List() ([]Entry, error) {
	des, err := os.ReadDir(r.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, d := range des {
		if strings.HasPrefix(d.Name(), ".") {
			continue
		}
		t, err := os.Readlink(filepath.Join(r.Dir, d.Name()))
		if err != nil {
			continue
		}
		out = append(out, Entry{Name: d.Name(), Target: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RevisionRoot is the root name for a build revision.
func RevisionRoot(revisionID string) string { return "rev-" + revisionID }

// PruneRevisions keeps the newest `keep` rev-* roots whose names carry the
// given project prefix and removes the rest. Revision ids are UUIDv7, so
// lexical order is creation order.
func (r Roots) PruneRevisions(project string, keep int) ([]string, error) {
	es, err := r.List()
	if err != nil {
		return nil, err
	}
	var revs []Entry
	for _, e := range es {
		if strings.HasPrefix(e.Name, "rev-"+project+"-") {
			revs = append(revs, e)
		}
	}
	if len(revs) <= keep {
		return nil, nil
	}
	var removed []string
	for _, e := range revs[:len(revs)-keep] {
		if err := r.Remove(e.Name); err != nil {
			return removed, err
		}
		removed = append(removed, e.Name)
	}
	return removed, nil
}
