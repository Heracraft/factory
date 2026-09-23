package cli

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxSyncFileBytes = 100 << 20 // 100 MB, 07-cli.md §5.5d "skip files over 100 MB with a warning"

// SyncOptions configures the sync step of `repose run` (07-cli.md §5.5).
type SyncOptions struct {
	StashRemote   bool
	DiscardRemote bool
	Exclude       []string
	// NoRemote is a project created with --name in a directory that has no
	// git remote: there are no remote-tracking refs to carry and no
	// origin to point at. The commits travel the same way as for every
	// other project (I-150).
	NoRemote bool
	// RemoteURL is the project's normalised remote ("github.com/a/b"); the
	// guest gets an `origin` pointing at it when it has none, the way
	// guestd's SetupProject names it (I-107), so an agent can push.
	RemoteURL string
}

// SyncSummary is what step 5e prints.
type SyncSummary struct {
	Modified   int
	Untracked  int
	Commits    int // commits the laptop sent that the guest did not have
	Detached   bool
	Diverged   bool // the guest's branch has commits the laptop does not; left alone
	Branch     string
	Head       string
	SkippedBig []string
}

// dirtyTreeError is 07-cli.md §6's exit 6, carrying the file list for the
// message in §5.5b.
type dirtyTreeError struct{ files []string }

func (e *dirtyTreeError) Error() string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "The guest's working tree has uncommitted changes (%d files):\n", len(e.files))
	for _, f := range e.files {
		_, _ = fmt.Fprintf(&b, "  %s\n", f)
	}
	b.WriteString("An agent may still be working. Re-run with --stash-remote (keeps them in `git stash`) or --discard-remote (throws them away), or `repose attach` to look first.")
	return b.String()
}

// guestProbe is what the first round trip learns about the guest's
// checkout.
type guestProbe struct {
	dirty     []string
	tips      []string // every commit a ref (or HEAD) in the guest points at
	hasOrigin bool
}

// probeScript creates the checkout if it is missing (an empty guest, or
// one whose SetupProject has not run), then reports the dirty list, the
// commits the guest has refs to, and whether it has an origin. One ssh.
func probeScript(slug string) string {
	return fmt.Sprintf(`set -e
d=~/%s
mkdir -p "$d"
cd "$d"
[ -d .git ] || git init -q
echo '#status'
git status --porcelain
echo '#tips'
git for-each-ref --format='%%(objectname)'
git rev-parse -q --verify HEAD || true
echo '#origin'
git remote get-url origin >/dev/null 2>&1 && echo yes || true
`, slug)
}

func parseProbe(out string) guestProbe {
	var p guestProbe
	section := ""
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		switch l {
		case "#status", "#tips", "#origin":
			section = l
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		switch section {
		case "#status":
			p.dirty = append(p.dirty, l)
		case "#tips":
			if t := strings.TrimSpace(l); !seen[t] {
				seen[t] = true
				p.tips = append(p.tips, t)
			}
		case "#origin":
			p.hasOrigin = strings.TrimSpace(l) == "yes"
		}
	}
	return p
}

// syncGuest runs the whole sync step against localRepoDir's git state
// (I-150): the laptop sends the commits itself, as a git bundle of what
// the guest lacks, so the guest never needs credentials for origin. Two
// ssh round trips: a probe, then one payload (bundle, diff, untracked
// tar) and one script that applies it.
func syncGuest(ctx context.Context, t sshTarget, localRepoDir, slug string, opts SyncOptions) (*SyncSummary, error) {
	if gitRepoRoot(localRepoDir) == "" {
		return nil, exitf(ExitUsage, "repose syncs your work through git, and %s is not a git checkout. Run `git init && git add -A && git commit -m init` there first, or pass --no-sync.", localRepoDir)
	}
	head, err := gitHeadCommit(localRepoDir)
	if err != nil {
		return nil, exitf(ExitUsage, "Your checkout has no commits yet, so there is nothing to sync. Commit once (`git add -A && git commit -m init`) and run again, or pass --no-sync.")
	}
	if shallow, _ := gitCmd(localRepoDir, "rev-parse", "--is-shallow-repository"); shallow == "true" {
		return nil, exitf(ExitUsage, "Your checkout is a shallow clone, so repose cannot send its history to the guest. Run `git fetch --unshallow` and try again, or pass --no-sync.")
	}
	branch, err := gitCurrentBranch(localRepoDir)
	if err != nil {
		return nil, stepFailed("read the current branch", err, "")
	}

	out, err := runSSH(ctx, t, probeScript(slug), nil)
	if err != nil {
		return nil, stepFailed("read the guest's checkout", err, "")
	}
	probe := parseProbe(string(out))
	if len(probe.dirty) > 0 && !opts.StashRemote && !opts.DiscardRemote {
		return nil, &exitError{code: ExitDirtyRemoteTree, msg: (&dirtyTreeError{files: probe.dirty}).Error()}
	}

	// What the guest should end up with: HEAD's commit, and, for a
	// project with a remote, the laptop's view of origin/<branch> so the
	// agent's `git status` and `git push` know where origin stands.
	track := ""
	if branch != "" && !opts.NoRemote {
		if sha, err := gitCmd(localRepoDir, "rev-parse", "-q", "--verify", "refs/remotes/origin/"+branch); err == nil {
			track = sha
		}
	}

	known, err := commitsKnownLocally(localRepoDir, probe.tips)
	if err != nil {
		return nil, stepFailed("compare commits with the guest", err, "")
	}
	wantRefs := []string{"HEAD"}
	if track != "" {
		wantRefs = append(wantRefs, "refs/remotes/origin/"+branch)
	}
	revs := append([]string(nil), wantRefs...)
	for _, k := range known {
		revs = append(revs, "^"+k)
	}
	countOut, err := gitCmdStdin(localRepoDir, strings.Join(revs, "\n")+"\n", "rev-list", "--count", "--stdin")
	if err != nil {
		return nil, stepFailed("count the commits to send", err, "")
	}
	summary := &SyncSummary{Branch: branch, Head: head}
	_, _ = fmt.Sscanf(strings.TrimSpace(countOut), "%d", &summary.Commits)

	payload, err := os.CreateTemp("", "repose-sync-*.tar")
	if err != nil {
		return nil, err
	}
	defer func() { _ = payload.Close(); _ = os.Remove(payload.Name()) }()
	tw := tar.NewWriter(payload)

	var bundleRefs []string
	if summary.Commits > 0 {
		bundle, err := os.CreateTemp("", "repose-bundle-*")
		if err != nil {
			return nil, err
		}
		bundlePath := bundle.Name()
		_ = bundle.Close()
		defer func() { _ = os.Remove(bundlePath) }()
		if _, err := gitCmdStdin(localRepoDir, strings.Join(revs, "\n")+"\n", "bundle", "create", "-q", bundlePath, "--stdin"); err != nil {
			return nil, stepFailed("pack your commits for the guest (git bundle)", err, "")
		}
		// A ref whose commit the guest already has is left out of the
		// bundle; fetch exactly the ones it carries.
		heads, err := gitCmd(localRepoDir, "bundle", "list-heads", bundlePath)
		if err != nil {
			return nil, stepFailed("read the bundle back", err, "")
		}
		bundleRefs = nil
		for _, l := range nonEmptyLines(heads) {
			if _, ref, ok := strings.Cut(l, " "); ok {
				bundleRefs = append(bundleRefs, ref)
			}
		}
		if err := tarAddFile(tw, "bundle", bundlePath); err != nil {
			return nil, err
		}
	}

	localDirty, err := gitTrackedDirty(localRepoDir)
	if err != nil {
		return nil, stepFailed("read your working tree", err, "")
	}
	summary.Modified = len(localDirty)
	diff, err := gitDiffBinary(localRepoDir)
	if err != nil {
		return nil, stepFailed("diff your working tree", err, "")
	}
	if strings.TrimSpace(diff) != "" {
		if err := tarAddBytes(tw, "diff", []byte(diff)); err != nil {
			return nil, err
		}
	}

	untracked, err := gitUntrackedFiles(localRepoDir)
	if err != nil {
		return nil, stepFailed("list your untracked files", err, "")
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
		if err := tarAddBytes(tw, "untracked.tar", buf); err != nil {
			return nil, err
		}
		summary.Untracked = len(untracked)
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if _, err := payload.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	script := applyScript(slug, head, branch, track, bundleRefs, len(bundleRefs) > 0, opts, probe)
	res, err := runSSH(ctx, t, script, payload)
	if err != nil {
		if se, ok := err.(*sshError); ok && (strings.Contains(se.Stderr, "patch does not apply") || strings.Contains(se.Stderr, "patch failed")) {
			return nil, stepFailed("apply your uncommitted changes in the guest", err, "Commit or stash them on the laptop and run again.")
		}
		return nil, stepFailed("sync your checkout to the guest", err, "")
	}
	for _, l := range strings.Split(string(res), "\n") {
		switch strings.TrimSpace(l) {
		case "#detached":
			summary.Detached = true
		case "#diverged":
			summary.Detached, summary.Diverged = true, true
		}
	}
	return summary, nil
}

// applyScript is the second round trip: unpack the payload, set the
// guest's tree aside if asked, fetch the bundle, move the refs, check
// out, and lay the diff and the untracked files on top.
func applyScript(slug, head, branch, track string, bundleRefs []string, hasBundle bool, opts SyncOptions, probe guestProbe) string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "set -e\ncd ~/%s\n", slug)
	b.WriteString("t=$(mktemp -d)\ntrap 'rm -rf \"$t\"' EXIT\ntar -x -C \"$t\"\n")
	if len(probe.dirty) > 0 {
		switch {
		case opts.StashRemote:
			b.WriteString("git stash push -q -u -m 'repose run'\n")
		case opts.DiscardRemote:
			b.WriteString("if git rev-parse -q --verify HEAD >/dev/null; then git reset -q --hard; fi\ngit clean -fdq\n")
		}
	}
	if hasBundle {
		_, _ = fmt.Fprintf(&b, "git fetch -q \"$t/bundle\" %s\n", strings.Join(bundleRefs, " "))
	}
	if track != "" {
		ref := shQuote("refs/remotes/origin/" + branch)
		_, _ = fmt.Fprintf(&b, "if ! cur=$(git rev-parse -q --verify %s) || git merge-base --is-ancestor \"$cur\" %s; then git update-ref %s %s; fi\n", ref, track, ref, track)
	}
	if !opts.NoRemote && !probe.hasOrigin && opts.RemoteURL != "" {
		if u := originURLFor(opts.RemoteURL); u != "" {
			_, _ = fmt.Fprintf(&b, "git remote add origin %s 2>/dev/null || true\n", shQuote(u))
		}
	}
	if branch == "" {
		_, _ = fmt.Fprintf(&b, "git checkout -q --detach %s\necho '#detached'\n", head)
	} else {
		br := shQuote(branch)
		_, _ = fmt.Fprintf(&b, `if cur=$(git rev-parse -q --verify refs/heads/%[1]s); then
  if git merge-base --is-ancestor "$cur" %[2]s; then git checkout -q -B %[1]s %[2]s; else git checkout -q --detach %[2]s; echo '#diverged'; fi
else
  git checkout -q -b %[1]s %[2]s
fi
`, br, head)
		if track != "" {
			_, _ = fmt.Fprintf(&b, "git branch -q --set-upstream-to=%s %s >/dev/null 2>&1 || true\n", shQuote("origin/"+branch), br)
		}
	}
	b.WriteString("if [ -s \"$t/diff\" ]; then git apply --index \"$t/diff\"; fi\n")
	b.WriteString("if [ -f \"$t/untracked.tar\" ]; then tar -x -f \"$t/untracked.tar\"; fi\n")
	return b.String()
}

// originURLFor is guestd's rule for the origin it sets (internal/guestd/
// project originURL, I-107): the api's normalised "host/owner/repo"
// becomes the SSH clone URL "git@host:owner/repo.git".
func originURLFor(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if strings.Contains(remote, "://") || strings.HasPrefix(remote, "git@") {
		return remote
	}
	host, path, ok := strings.Cut(remote, "/")
	if !ok || host == "" || path == "" {
		return ""
	}
	return "git@" + host + ":" + strings.TrimSuffix(path, ".git") + ".git"
}

// commitsKnownLocally filters the guest's tips to the ones this checkout
// has as commits (or tags), the only ones `--not` can use.
func commitsKnownLocally(dir string, tips []string) ([]string, error) {
	if len(tips) == 0 {
		return nil, nil
	}
	out, err := gitCmdStdin(dir, strings.Join(tips, "\n")+"\n", "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return nil, err
	}
	var known []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && (f[1] == "commit" || f[1] == "tag") {
			known = append(known, f[0])
		}
	}
	return known, nil
}

// gitCmdStdin is gitCmd with stdin, for the --stdin forms that keep long
// rev lists off the command line.
func gitCmdStdin(dir, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s", msg)
	}
	return out.String(), nil
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
		hdr := &tar.Header{Name: filepath.ToSlash(f), Mode: int64(info.Mode().Perm()), Size: int64(len(b))}
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

func tarAddBytes(tw *tar.Writer, name string, b []byte) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(b))}); err != nil {
		return err
	}
	_, err := tw.Write(b)
	return err
}

func tarAddFile(tw *tar.Writer, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: info.Size()}); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func (s *SyncSummary) String() string {
	line := fmt.Sprintf("Synced: %d modified, %d untracked", s.Modified, s.Untracked)
	switch {
	case s.Commits == 1:
		line += " (1 new commit)"
	case s.Commits > 1:
		line += fmt.Sprintf(" (%d new commits)", s.Commits)
	}
	return line
}

// Warnings are the stderr lines that go with the summary.
func (s *SyncSummary) Warnings() []string {
	var w []string
	short := s.Head
	if len(short) > 7 {
		short = short[:7]
	}
	if s.Diverged {
		w = append(w, fmt.Sprintf("The guest's %s has commits your laptop does not have; it was left as it is and the guest is on %s, detached. Push them from the guest (or `repose attach` to look) and pull on the laptop.", s.Branch, short))
	}
	for _, f := range s.SkippedBig {
		w = append(w, fmt.Sprintf("Skipped %s: over 100 MB.", f))
	}
	return w
}
