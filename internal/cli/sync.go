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
	// BeforeApply runs between the probe and the apply with the guest's
	// carry markers: `run` sends the tool logins and the carry there, so
	// the checkout lands in a guest whose git already knows the user
	// (I-150) and the carry skips what the guest already has (I-195..
	// I-197) without a round trip of its own.
	BeforeApply func(markers map[string]string) error
	// Env is the laptop's gitignored .env files (I-197), written after the
	// checkout in the apply's own ssh, unless the guest's marker says it
	// has exactly these.
	Env []envFile
	// EnvLater, when set, is called after the probe for the .env files in
	// place of Env: `run` lists them (a walk of every ignored file) while
	// the probe's ssh is in flight rather than before it.
	EnvLater func() []envFile
}

func (o SyncOptions) envFiles() []envFile {
	if o.EnvLater != nil {
		return o.EnvLater()
	}
	return o.Env
}

// SyncSummary is what step 5e prints.
type SyncSummary struct {
	Modified  int
	Untracked int
	Commits   int // commits the laptop sent that the guest did not have
	Detached  bool
	Diverged  bool // the guest's branch has commits the laptop does not; left alone
	// StashedLastSync: the guest still held the previous sync's changes
	// and nothing else, and they were stashed ("repose run: last sync")
	// before this sync's were laid down (I-210).
	StashedLastSync bool
	Branch          string
	Head            string
	SkippedBig      []string
	// SkippedDirs are dependency and cache directories left behind
	// (defaultSkipDirs); SkippedCap counts files past maxUntrackedBytes.
	SkippedDirs []string
	SkippedCap  int
	// EnvFiles counts the .env files written; EnvKept names the ones the
	// guest kept because its copy was newer (I-197).
	EnvFiles int
	EnvKept  []string
	// ClonedFrom is the host the guest cloned from on a first sync
	// (I-203); CloneFailed is why it could not, when it tried.
	ClonedFrom  string
	CloneFailed string
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
	dirty []string
	// syncedOnly is a dirty tree that is exactly what the last sync left
	// (I-210): the laptop's own diff and untracked files, no agent work.
	syncedOnly bool
	tips       []string // every commit a ref (or HEAD) in the guest points at
	hasOrigin  bool
	markers    map[string]string // the carry's markers (carry.go)
}

// syncedFP holds the shell functions every dirtiness judgement of the
// sync goes through (I-210).
//
// repose_git is git with the settings a carried laptop config could turn
// against the sync forced back: status.showUntrackedFiles=no would hide
// the untracked files the sync wrote, and submodule.recurse=true would
// make a stash or reset reach into a submodule. repose_dirty is the dirty
// list, submodules included.
//
// repose_fp fingerprints the checkout as it stands: HEAD and the tree
// `git add -A` would record (index, working tree and every untracked file
// that is not ignored), built in a copy of the index so the real one is
// untouched and only changed files are hashed, and with a throwaway
// object directory (the real one as its alternate) so a probe leaves no
// objects behind. A submodule is only its commit in that tree, so an edit
// inside one would not change it: any submodule change makes the
// fingerprint "failed", which never matches. The apply stores it after
// laying down the laptop's diff and untracked files; the next probe
// compares, so the tree the sync itself made dirty is not taken for an
// agent's work.
const syncedFP = `repose_git() { git -c status.showUntrackedFiles=normal -c submodule.recurse=false "$@"; }
repose_dirty() { repose_git status --porcelain --ignore-submodules=none; }
repose_fp() {
  if repose_git status --porcelain=v2 --ignore-submodules=none | grep -q '^[12u] .. S'; then echo failed; return; fi
  i=$(mktemp)
  o=$(mktemp -d)
  x=$(git rev-parse --git-path index)
  a=$(cd "$(git rev-parse --git-path objects)" && pwd)
  if [ -f "$x" ]; then cp "$x" "$i"; else rm -f "$i"; fi
  if GIT_INDEX_FILE=$i GIT_OBJECT_DIRECTORY=$o GIT_ALTERNATE_OBJECT_DIRECTORIES=$a repose_git add -A >/dev/null 2>&1 &&
     tr=$(GIT_INDEX_FILE=$i GIT_OBJECT_DIRECTORY=$o GIT_ALTERNATE_OBJECT_DIRECTORIES=$a git write-tree 2>/dev/null); then
    printf '%s %s\n' "$(git rev-parse -q --verify HEAD || echo none)" "$tr"
  else
    echo failed
  fi
  rm -rf "$i" "$o"
}
repose_synced=$(git rev-parse --git-path repose-synced)
`

// probeScript creates the checkout if it is missing (an empty guest, or
// one whose SetupProject has not run), then reports the dirty list,
// whether that dirt is only what the last sync wrote (I-210), the
// commits the guest has refs to, and whether it has an origin. One ssh.
func probeScript(slug string) string {
	return fmt.Sprintf(`set -e
d=~/%s
mkdir -p "$d"
cd "$d"
[ -d .git ] || git init -q
%s
%s
st=$(repose_dirty)
echo '#status'
[ -z "$st" ] || printf '%%s\n' "$st"
echo '#synced'
if [ -n "$st" ] && [ -s "$repose_synced" ] && [ "$(repose_fp)" = "$(cat "$repose_synced")" ]; then echo yes; fi
echo '#tips'
git for-each-ref --format='%%(objectname)'
git rev-parse -q --verify HEAD || true
echo '#origin'
git remote get-url origin >/dev/null 2>&1 && echo yes || true
%s`, slug, syncedFP, envPathsCheck, markerScript())
}

func parseProbe(out string) guestProbe {
	p := guestProbe{markers: parseMarkers(out)}
	section := ""
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "#marker ") {
			continue
		}
		if l == "#envmissing" {
			delete(p.markers, "env") // a written file is gone: send the set again
			continue
		}
		switch l {
		case "#status", "#synced", "#tips", "#origin":
			section = l
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		switch section {
		case "#status":
			p.dirty = append(p.dirty, l)
		case "#synced":
			p.syncedOnly = strings.TrimSpace(l) == "yes"
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
	if len(probe.dirty) > 0 && !probe.syncedOnly && !opts.StashRemote && !opts.DiscardRemote {
		return nil, &exitError{code: ExitDirtyRemoteTree, msg: (&dirtyTreeError{files: probe.dirty}).Error()}
	}
	if opts.BeforeApply != nil {
		if err := opts.BeforeApply(probe.markers); err != nil {
			return nil, err
		}
	}
	// The first sync of a large GitHub repository clones in the guest,
	// after the credentials (gh's helper) are in place (I-203).
	var cloned, cloneFailed string
	if len(probe.tips) == 0 && !opts.NoRemote {
		if url := hybridCloneURL(opts.RemoteURL); url != "" && gitPackKiB(localRepoDir) >= hybridThresholdKiB {
			tips, ok, why, err := hybridFetch(ctx, t, slug, url, laptopBases(localRepoDir))
			if err != nil {
				return nil, err
			}
			if ok {
				probe.tips = tips
				cloned = remoteHost(opts.RemoteURL)
			} else {
				cloneFailed = why
			}
		}
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
	summary := &SyncSummary{Branch: branch, Head: head, ClonedFrom: cloned, CloneFailed: cloneFailed}
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
	untracked, skipped, skippedDirs, skippedCap, err := filterUntracked(localRepoDir, untracked, opts.Exclude)
	if err != nil {
		return nil, err
	}
	summary.SkippedBig = skipped
	summary.SkippedDirs = skippedDirs
	summary.SkippedCap = skippedCap
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
	envScript, err := addEnvToApply(tw, opts.envFiles(), probe.markers)
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if _, err := payload.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	script := applyScript(slug, head, branch, track, bundleRefs, len(bundleRefs) > 0, opts, probe) + envScript + recordSyncedScript
	res, err := runSSH(ctx, t, script, payload)
	if err != nil {
		if se, ok := err.(*sshError); ok && strings.Contains(se.Stderr, syncedChanged) {
			// An agent wrote between the probe and the apply: its work
			// now, not the last sync's, so it is refused like any other.
			return nil, &exitError{code: ExitDirtyRemoteTree, msg: (&dirtyTreeError{files: probe.dirty}).Error()}
		}
		if se, ok := err.(*sshError); ok && (strings.Contains(se.Stderr, "patch does not apply") || strings.Contains(se.Stderr, "patch failed")) {
			return nil, stepFailed("apply your uncommitted changes in the guest", err, "Commit or stash them on the laptop and run again.")
		}
		return nil, stepFailed("sync your checkout to the guest", err, "")
	}
	for _, l := range strings.Split(string(res), "\n") {
		l = strings.TrimSpace(l)
		switch l {
		case "#detached":
			summary.Detached = true
		case "#diverged":
			summary.Detached, summary.Diverged = true, true
		case "#stashedsync":
			summary.StashedLastSync = true
		}
		if rest, ok := strings.CutPrefix(l, "#kept "); ok {
			summary.EnvKept = append(summary.EnvKept, rest)
		}
		if rest, ok := strings.CutPrefix(l, "#envfiles "); ok {
			_, _ = fmt.Sscanf(rest, "%d", &summary.EnvFiles)
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
	b.WriteString(syncedFP)
	b.WriteString("t=$(mktemp -d)\ntrap 'rm -rf \"$t\"' EXIT\ntar -x -C \"$t\"\n")
	if len(probe.dirty) > 0 {
		switch {
		case opts.DiscardRemote:
			b.WriteString("if git rev-parse -q --verify HEAD >/dev/null; then repose_git reset -q --hard; fi\nrepose_git clean -fdq\n")
		case opts.StashRemote:
			b.WriteString("repose_git stash push -q -u -m 'repose run'\n")
		case probe.syncedOnly:
			// The last sync's own changes, which the laptop still has (or
			// has replaced): stashed, not thrown away, so a write that
			// lands after this check is still recoverable; and not at all
			// if something moved since the probe looked.
			_, _ = fmt.Fprintf(&b, "if [ \"$(repose_fp)\" != \"$(cat \"$repose_synced\" 2>/dev/null)\" ]; then echo %s >&2; exit 3; fi\n", shQuote(syncedChanged))
			b.WriteString("repose_git stash push -q -u -m 'repose run: last sync'\necho '#stashedsync'\n" + pruneSyncStashes)
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

// syncStashKeep is how many "repose run: last sync" stashes the guest
// keeps; one is made per run from a dirty laptop tree (I-210).
const syncStashKeep = 10

// pruneSyncStashes drops every "repose run: last sync" stash past the
// newest syncStashKeep, oldest first so the indices of the ones still to
// drop do not move. It matches the whole subject git records for
// `stash push -m` ("On <branch>: <message>", a branch name has no colon),
// so the user's stashes and --stash-remote's "repose run" are never
// touched.
var pruneSyncStashes = fmt.Sprintf(`n=0
drop=""
while IFS=' ' read -r ref subj; do
  case $subj in
    "On "*": repose run: last sync")
      br=${subj#On }; br=${br%%": repose run: last sync"}
      case $br in *:*) continue ;; esac
      n=$((n+1))
      [ "$n" -gt %d ] && drop="$ref $drop"
      ;;
  esac
done <<EOF
$(git stash list --format='%%gd %%gs')
EOF
for ref in $drop; do git stash drop -q "$ref"; done
`, syncStashKeep)

// syncedChanged is the apply's stderr when the tree it was told was the
// last sync's own changed after the probe.
const syncedChanged = "repose: the guest's tree changed since the sync looked at it"

// recordSyncedScript ends the apply: when the sync left the tree dirty
// (the laptop's diff, its untracked files), the fingerprint of that
// state is stored in the checkout's .git, for the next probe (I-210);
// a clean tree needs none.
const recordSyncedScript = `if [ -n "$(repose_dirty | head -n 1)" ]; then
  fp=$(repose_fp)
  if [ "$fp" = failed ]; then rm -f "$repose_synced"; else printf '%s\n' "$fp" > "$repose_synced"; fi
else
  rm -f "$repose_synced"
fi
`

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

// maxUntrackedBytes caps what one sync sends of untracked files in total:
// past it the rest is skipped with a warning naming the biggest
// directories, rather than packing gigabytes into memory (owner's run on
// 2026-09-23, a pnpm tree under an un-ignored cms/node_modules).
const maxUntrackedBytes = 500 << 20

// defaultSkipDirs are directories that never travel, wherever they sit in
// the tree and whether or not .gitignore mentions them: dependency trees
// and build caches the guest recreates itself (an install there is
// faster than shipping them, and they are full of symlinks and
// platform-specific binaries that would be wrong in the guest anyway).
var defaultSkipDirs = map[string]bool{
	"node_modules": true, ".pnpm-store": true, "bower_components": true,
	".venv": true, "venv": true, "__pycache__": true, ".mypy_cache": true, ".pytest_cache": true, ".ruff_cache": true, ".tox": true,
	".turbo": true, ".next": true, ".nuxt": true, ".svelte-kit": true, ".parcel-cache": true, ".angular": true,
	".gradle": true, ".terraform": true, ".direnv": true,
}

// skippedDir is the shortest leading directory of f that is a default
// skip ("cms/node_modules" for "cms/node_modules/.pnpm/x/y"), or "".
func skippedDir(f string) string {
	parts := strings.Split(filepath.ToSlash(f), "/")
	for i, part := range parts[:len(parts)-1] {
		if defaultSkipDirs[part] {
			return strings.Join(parts[:i+1], "/")
		}
	}
	return ""
}

// filterUntracked drops default-skipped directories (named once each in
// skippedDirs), anything sync.exclude matches, files over the size cap
// (named in skippedBig), and everything past the total cap (skippedCap
// counts them). Directories and anything that is not a regular file or
// a symlink are dropped silently; symlinks are kept and travel as links.
func filterUntracked(root string, files, exclude []string) (kept, skippedBig, skippedDirs []string, skippedCap int, err error) {
	seenDir := map[string]bool{}
	var total int64
	for _, f := range files {
		if d := skippedDir(f); d != "" {
			if !seenDir[d] {
				seenDir[d] = true
				skippedDirs = append(skippedDirs, d)
			}
			continue
		}
		if matchesAny(exclude, f) {
			continue
		}
		info, statErr := os.Lstat(filepath.Join(root, f))
		if statErr != nil {
			continue // gone between listing and syncing; nothing to send
		}
		mode := info.Mode()
		if mode&os.ModeSymlink == 0 && !mode.IsRegular() {
			continue
		}
		if mode.IsRegular() && info.Size() > maxSyncFileBytes {
			skippedBig = append(skippedBig, f)
			continue
		}
		if total+info.Size() > maxUntrackedBytes {
			skippedCap++
			continue
		}
		total += info.Size()
		kept = append(kept, f)
	}
	return kept, skippedBig, skippedDirs, skippedCap, nil
}

// matchesAny reports whether a sync.exclude pattern matches the path, its
// base name, or any leading directory of it (so "dist" and "web/dist"
// both exclude everything under web/dist).
func matchesAny(patterns []string, path string) bool {
	path = filepath.ToSlash(path)
	parts := strings.Split(path, "/")
	for _, p := range patterns {
		p = strings.TrimSuffix(filepath.ToSlash(p), "/")
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
		if ok, _ := filepath.Match(p, parts[len(parts)-1]); ok {
			return true
		}
		for i := range parts[:len(parts)-1] {
			if ok, _ := filepath.Match(p, parts[i]); ok {
				return true
			}
			if ok, _ := filepath.Match(p, strings.Join(parts[:i+1], "/")); ok {
				return true
			}
		}
	}
	return false
}

// tarFiles packs the files filterUntracked kept. A symlink travels as a
// symlink (pnpm's node_modules is made of them, and reading one that
// points at a directory as a file is what failed the owner's sync); a
// file that turned into something else since the listing is skipped.
func tarFiles(root string, files []string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, f := range files {
		path := filepath.Join(root, f)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		name := filepath.ToSlash(f)
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				continue
			}
			if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: name, Linkname: filepath.ToSlash(target), Mode: 0o777, ModTime: info.ModTime()}); err != nil {
				return nil, err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue // unreadable (permissions, a race): one file must not sink the whole sync
		}
		hdr := &tar.Header{Name: name, Mode: int64(info.Mode().Perm()), Size: int64(len(b)), ModTime: info.ModTime()}
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
	case s.EnvFiles == 1:
		line += ", 1 env file"
	case s.EnvFiles > 1:
		line += fmt.Sprintf(", %d env files", s.EnvFiles)
	}
	switch {
	case s.Commits == 1:
		line += " (1 new commit)"
	case s.Commits > 1:
		line += fmt.Sprintf(" (%d new commits)", s.Commits)
	}
	if s.ClonedFrom != "" {
		line += ", history cloned from " + s.ClonedFrom
	}
	if s.StashedLastSync {
		line += "; the last sync's changes stashed in the guest"
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
	if s.CloneFailed != "" {
		w = append(w, fmt.Sprintf("The guest could not clone from GitHub (%s), so the history was sent from your laptop instead.", s.CloneFailed))
	}
	for _, k := range s.EnvKept {
		w = append(w, fmt.Sprintf("Kept the guest's %s: it is newer than the laptop's.", k))
	}
	if s.Diverged {
		w = append(w, fmt.Sprintf("The guest's %s has commits your laptop does not have; it was left as it is and the guest is on %s, detached. Push them from the guest (or `repose attach` to look) and pull on the laptop.", s.Branch, short))
	}
	for _, f := range s.SkippedBig {
		w = append(w, fmt.Sprintf("Skipped %s: over 100 MB.", f))
	}
	if len(s.SkippedDirs) > 0 {
		w = append(w, fmt.Sprintf("Not sent: %s (dependencies and caches; install them in the guest). Add them to .gitignore to keep them out of git too.", strings.Join(s.SkippedDirs, ", ")))
	}
	if s.SkippedCap > 0 {
		w = append(w, fmt.Sprintf("Skipped %d untracked files past the 500 MB limit for one sync. Commit what matters, or add large directories to .gitignore or `sync.exclude`.", s.SkippedCap))
	}
	return w
}
