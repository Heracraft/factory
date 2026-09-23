package cli

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// gitCmd runs git in dir and returns trimmed stdout. A non-zero exit with
// no meaningful stderr is folded into the error.
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// gitRepoRoot returns the repo root containing dir, or "" if dir is not
// inside a git repository.
func gitRepoRoot(dir string) string {
	root, err := gitCmd(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

// gitRemoteOrigin returns the normalised "origin" remote for the repo
// containing dir, or "" if there is no repo or no such remote.
func gitRemoteOrigin(dir string) string {
	root := gitRepoRoot(dir)
	if root == "" {
		return ""
	}
	url, err := gitCmd(root, "remote", "get-url", "origin")
	if err != nil || url == "" {
		return ""
	}
	return normalizeRemote(url)
}

func gitHeadCommit(dir string) (string, error) { return gitCmd(dir, "rev-parse", "HEAD") }

func gitCurrentBranch(dir string) (string, error) {
	name, err := gitCmd(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if name == "HEAD" {
		return "", nil // detached
	}
	return name, nil
}

// gitTrackedDirty is the local "dirty list" of 07-cli.md §5.5a: modified
// tracked files only, since untracked ones are counted separately by
// gitUntrackedFiles and the two must not double up in the sync summary.
func gitTrackedDirty(dir string) ([]string, error) {
	out, err := gitCmd(dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

// gitUntrackedFiles lists files git would add, respecting .gitignore.
func gitUntrackedFiles(dir string) ([]string, error) {
	out, err := gitCmd(dir, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func nonEmptyLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// gitDiffBinary is "git diff HEAD --binary", the patch the run sequence
// applies remotely.
func gitDiffBinary(dir string) (string, error) {
	cmd := exec.Command("git", "diff", "HEAD", "--binary")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
