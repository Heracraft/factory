package cli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxSyncFileBytes = 100 << 20 // 100 MB, 07-cli.md §5.5d "skip files over 100 MB with a warning"

// SyncOptions configures the sync step of `repose run` (07-cli.md §5.5).
type SyncOptions struct {
	StashRemote   bool
	DiscardRemote bool
	Exclude       []string
	// AskPush is called when the local HEAD commit is not on origin; it
	// should push (or not) and report whether it did.
	AskPush func(commit, branch string) (pushed bool, err error)
}

// SyncSummary is what step 5e prints.
type SyncSummary struct {
	Modified   int
	Untracked  int
	Detached   bool
	Branch     string
	SkippedBig []string
}

// dirtyTreeError is 07-cli.md §6's exit 6, carrying the file list for the
// message in §5.5b.
type dirtyTreeError struct{ files []string }

func (e *dirtyTreeError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "The guest's working tree has uncommitted changes (%d files):\n", len(e.files))
	for _, f := range e.files {
		fmt.Fprintf(&b, "  %s\n", f)
	}
	b.WriteString("An agent may still be working. Re-run with --stash-remote (keeps them in `git stash`) or --discard-remote (throws them away), or `repose attach` to look first.")
	return b.String()
}

// syncGuest runs the whole sync step against localRepoDir's git state.
func syncGuest(ctx context.Context, t sshTarget, localRepoDir, slug string, opts SyncOptions) (*SyncSummary, error) {
	dirtyLines, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git status --porcelain", slug), nil)
	if err != nil {
		return nil, err
	}
	remoteDirty := nonEmptyLines(string(dirtyLines))
	if len(remoteDirty) > 0 {
		switch {
		case opts.StashRemote:
			if _, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git stash push -u -m 'repose run'", slug), nil); err != nil {
				return nil, err
			}
		case opts.DiscardRemote:
			if _, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git reset --hard && git clean -fd", slug), nil); err != nil {
				return nil, err
			}
		default:
			return nil, &exitError{code: ExitDirtyRemoteTree, msg: (&dirtyTreeError{files: remoteDirty}).Error()}
		}
	}

	head, err := gitHeadCommit(localRepoDir)
	if err != nil {
		return nil, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	branch, err := gitCurrentBranch(localRepoDir)
	if err != nil {
		return nil, fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}

	if _, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git fetch origin", slug), nil); err != nil {
		return nil, err
	}
	if err := runSSHOK(ctx, t, fmt.Sprintf("cd ~/%s && git cat-file -e %s", slug, head)); err != nil {
		if opts.AskPush == nil {
			return nil, fmt.Errorf("commit %s is not on origin", head)
		}
		pushed, err := opts.AskPush(head, branch)
		if err != nil {
			return nil, err
		}
		if !pushed {
			return nil, fmt.Errorf("commit %s is not on origin and was not pushed", head)
		}
		if _, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git fetch origin", slug), nil); err != nil {
			return nil, err
		}
	}

	if _, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git checkout --detach %s", slug, head), nil); err != nil {
		return nil, err
	}
	summary := &SyncSummary{Detached: true, Branch: branch}
	if branch != "" {
		// Only ride the local branch if it already sits at H: git fetch
		// only moves remote-tracking refs, so a stale local branch would
		// otherwise silently undo the checkout just done.
		branchHead, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git rev-parse %s", slug, branch), nil)
		if err == nil && strings.TrimSpace(string(branchHead)) == head {
			if err := runSSHOK(ctx, t, fmt.Sprintf("cd ~/%s && git checkout %s", slug, branch)); err == nil {
				summary.Detached = false
			}
		}
	}

	diff, err := gitDiffBinary(localRepoDir)
	if err != nil {
		return nil, fmt.Errorf("git diff HEAD --binary: %w", err)
	}
	if strings.TrimSpace(diff) != "" {
		if _, err := runSSH(ctx, t, fmt.Sprintf("cd ~/%s && git apply --index", slug), strings.NewReader(diff)); err != nil {
			return nil, fmt.Errorf("applying local diff: %w", err)
		}
		summary.Modified = strings.Count(diff, "\ndiff --git ")
		if strings.HasPrefix(diff, "diff --git ") {
			summary.Modified++
		}
	}

	untracked, err := gitUntrackedFiles(localRepoDir)
	if err != nil {
		return nil, fmt.Errorf("git ls-files --others: %w", err)
	}
	untracked, skipped, err := filterUntracked(localRepoDir, untracked, opts.Exclude)
	if err != nil {
		return nil, err
	}
	summary.SkippedBig = skipped
	if len(untracked) > 0 {
		buf, err := tarFiles(localRepoDir, untracked)
		if err != nil {
			return nil, err
		}
		if _, err := runSSH(ctx, t, fmt.Sprintf("tar -x -C ~/%s", slug), bytes.NewReader(buf)); err != nil {
			return nil, fmt.Errorf("copying untracked files: %w", err)
		}
		summary.Untracked = len(untracked)
	}
	return summary, nil
}

// filterUntracked drops files over the size cap (with their names
// returned separately for a warning) and anything sync.exclude matches.
func filterUntracked(root string, files, exclude []string) (kept, skippedBig []string, err error) {
	for _, f := range files {
		if matchesAny(exclude, f) {
			continue
		}
		info, statErr := os.Stat(filepath.Join(root, f))
		if statErr != nil {
			continue // gone between listing and syncing; nothing to send
		}
		if info.Size() > maxSyncFileBytes {
			skippedBig = append(skippedBig, f)
			continue
		}
		kept = append(kept, f)
	}
	return kept, skippedBig, nil
}

func matchesAny(patterns []string, path string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
		if ok, _ := filepath.Match(p, filepath.Base(path)); ok {
			return true
		}
	}
	return false
}

func tarFiles(root string, files []string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range files {
		path := filepath.Join(root, f)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		hdr := &tar.Header{Name: f, Mode: int64(info.Mode().Perm()), Size: int64(len(b))}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(b); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *SyncSummary) String() string {
	return fmt.Sprintf("Synced: %d modified, %d untracked", s.Modified, s.Untracked)
}
