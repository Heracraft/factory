package cli

import (
	"archive/tar"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The .env part of the carry (I-197): gitignored `.env` and `.env.*` files
// anywhere in the checkout outside dependency directories, up to 1 MB
// each, laptop to guest over the sync's own ssh, mode 0600, never through
// the api. As with tool logins, a guest copy newer than the laptop's is
// kept and named. They are written at the end of the sync's apply, after
// the checkout, so the guest's .gitignore already covers them when an
// agent next stashes or cleans.
//
// Their contents never reach a log line; the CLI prints the count, and the
// name of a file it kept.

const envFileCap = 1 << 20

type envFile struct {
	Rel   string // slash-separated, relative to the checkout
	Body  []byte
	Mtime int64
}

// isEnvName is `.env` or `.env.<anything>`.
func isEnvName(base string) bool {
	return base == ".env" || strings.HasPrefix(base, ".env.")
}

// buildEnvCarry lists the checkout's ignored, untracked files (collapsing
// ignored directories, so a node_modules tree costs one line) and keeps
// the .env files among them, in every checked-out submodule too (I-263),
// with paths relative to the superproject.
func buildEnvCarry(repoDir string) ([]envFile, error) {
	files, err := envFilesIn(repoDir, "")
	if err != nil {
		return nil, err
	}
	for _, sub := range populatedSubmodules(repoDir) {
		more, err := envFilesIn(repoDir, sub.Path)
		if err != nil {
			return nil, err
		}
		files = append(files, more...)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

// envFilesIn lists the .env files of the repository at top/sub ("" for
// top itself), as paths relative to top.
func envFilesIn(top, sub string) ([]envFile, error) {
	repoDir := filepath.Join(top, filepath.FromSlash(sub))
	out, err := gitCmdStdin(repoDir, "", "ls-files", "--others", "--ignored", "--exclude-standard", "--directory", "-z")
	if err != nil {
		return nil, err
	}
	var files []envFile
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" || strings.HasSuffix(rel, "/") || !isEnvName(path.Base(rel)) {
			continue
		}
		if skippedDir(rel) != "" || strings.HasPrefix(rel, "../") || path.IsAbs(rel) {
			continue
		}
		p := filepath.Join(repoDir, filepath.FromSlash(rel))
		info, err := os.Lstat(p)
		if err != nil || !info.Mode().IsRegular() || info.Size() > envFileCap {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		files = append(files, envFile{Rel: path.Join(sub, rel), Body: b, Mtime: info.ModTime().Unix()})
	}
	return files, nil
}

// envHash is the marker for a set of .env files: names, times and bytes.
func envHash(files []envFile) string {
	var parts [][]byte
	for _, f := range files {
		parts = append(parts, []byte(f.Rel), []byte(fmt.Sprint(f.Mtime)), f.Body)
	}
	return carryHash(parts...)
}

// envPathsCheck is the probe's half of the env marker. The last carry
// listed its files in ~/.repose/env-paths as "<mtime> <path>", the mtime
// each had when the carry ended. When one is gone from the checkout
// (deleted in the guest, or a restore of an older snapshot) the probe
// says #envmissing and the marker is not trusted, so the set is sent
// again. When one is newer than recorded (edited in the guest while the
// laptop's copy did not change) it says "#envnewer <path>" and records
// the new mtime, so the laptop names the kept file once per change, not
// on every run (15-dev-ergonomics §6, DECISIONS I-215). A line without
// an mtime is the older paths-only shape; it is read as a path and
// rewritten with one. Run in the checkout.
const envPathsCheck = `if [ -f ~/.repose/env-paths ]; then
  : > ~/.repose/env-paths.new
  while IFS= read -r line; do
    m=${line%% *}; p=${line#* }
    case $m in ''|*[!0-9]*) m=; p=$line ;; esac
    if [ ! -e "$p" ]; then echo '#envmissing'; continue; fi
    now=$(stat -c %Y "$p")
    if [ -n "$m" ] && [ "$now" -gt "$m" ]; then printf '#envnewer %s\n' "$p"; fi
    printf '%s %s\n' "$now" "$p" >> ~/.repose/env-paths.new
  done < ~/.repose/env-paths
  mv -f ~/.repose/env-paths.new ~/.repose/env-paths
fi
`

// addEnvToApply puts the files in the apply's tar and returns the shell
// that writes them, or "" when the guest already has this exact set.
// The paths travel in a file, never on a command line.
func addEnvToApply(tw *tar.Writer, files []envFile, markers map[string]string) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	hash := envHash(files)
	if markers != nil && markers["env"] == hash {
		return "", nil
	}
	var list strings.Builder
	for i, f := range files {
		if err := tarAddBytes(tw, fmt.Sprintf("env/%d", i), f.Body); err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(&list, "%d %d %s\n", i, f.Mtime, f.Rel)
	}
	if err := tarAddBytes(tw, "env/list", []byte(list.String())); err != nil {
		return "", err
	}
	return `n=0
while IFS= read -r line; do
  i=${line%% *}; rest=${line#* }; m=${rest%% *}; p=${rest#* }
  if [ -e "$p" ] && [ "$(stat -c %Y "$p")" -gt "$m" ]; then echo "#kept $p"; continue; fi
  mkdir -p "$(dirname "$p")"
  install -m 0600 "$t/env/$i" "$p"
  touch -d "@$m" "$p"
  n=$((n+1))
done < "$t/env/list"
echo "#envfiles $n"
mkdir -p ~/.repose
cut -d' ' -f3- "$t/env/list" | while IFS= read -r p; do
  if [ -e "$p" ]; then printf '%s %s\n' "$(stat -c %Y "$p")" "$p"; fi
done > ~/.repose/env-paths
` + setMarker("env", hash), nil
}
