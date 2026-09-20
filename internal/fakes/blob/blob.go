// Package blob is the in-memory snapshot store the api's expiry tests use:
// it records deletions and can be told to fail.
package blob

import (
	"context"
	"errors"
	"sync"
)

// Fake records blob operations.
type Fake struct {
	mu      sync.Mutex
	Blobs   map[string]int64
	Deleted []string
	Fail    error
}

// New returns an empty store.
func New() *Fake { return &Fake{Blobs: map[string]int64{}} }

// Put registers a blob (what a snapshot upload would have created).
func (f *Fake) Put(path string, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Blobs[path] = size
}

// Delete implements snapshots.BlobStore. Missing blobs are not errors,
// matching the real store's idempotent delete.
func (f *Fake) Delete(ctx context.Context, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	delete(f.Blobs, path)
	f.Deleted = append(f.Deleted, path)
	return nil
}

// Exists reports whether a blob is present.
func (f *Fake) Exists(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.Blobs[path]
	return ok
}

// ErrDown simulates an unreachable store.
var ErrDown = errors.New("fake blob store: unavailable")
