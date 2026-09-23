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
// the .env files among them.
func buildEnvCarry(repoDir string) ([]envFile, error) {
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
		files = append(files, envFile{Rel: rel, Body: b, Mtime: info.ModTime().Unix()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
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
` + setMarker("env", hash), nil
}
