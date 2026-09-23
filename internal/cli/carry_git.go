package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The git part of the carry (I-195): the laptop's effective global git
// config for this checkout, minus a denylist, written whole to the
// guest's ~/.config/git/repose-carried and included from ~/.gitconfig
// ahead of the guest's own keys, so a key set by hand in the guest wins.

// gitCarried is where the carried config lands in the guest, and the
// include line's path (git expands the ~ itself).
const gitCarried = "~/.config/git/repose-carried"

// gitEntry is one `key value` of `git config --list -z`. Keys come out of
// git with the section and name lowercased and the subsection as written.
type gitEntry struct {
	Key      string
	Value    string
	HasValue bool // false for the bare `name` form, which means true
}

// readGitConfig is `git config --global --list --includes -z` run in the
// checkout, so an `includeIf "gitdir:..."` that matches it is flattened
// in (a work email picked by the directory is the email in the guest).
// No global config at all is an empty list, not an error.
func readGitConfig(repoDir string) ([]gitEntry, error) {
	cmd := exec.Command("git", "config", "--global", "--list", "--includes", "-z")
	cmd.Dir = repoDir
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && out.Len() == 0 {
			return nil, nil // no global file
		}
		return nil, fmt.Errorf("git config: %s", strings.TrimSpace(stderr.String()))
	}
	return parseGitConfigZ(out.Bytes()), nil
}

// parseGitConfigZ splits -z output: NUL-terminated records of the key,
// then a newline and the value when there is one.
func parseGitConfigZ(b []byte) []gitEntry {
	var es []gitEntry
	for _, rec := range bytes.Split(b, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		k, v, has := bytes.Cut(rec, []byte{'\n'})
		es = append(es, gitEntry{Key: string(k), Value: string(v), HasValue: has})
	}
	return es
}

// gitDenied is I-195's denylist: keys that name something only the
// laptop has (keychains, ssh binaries, signing agents, proxies, GUI
// tools, laptop paths), or that would break the guest's own git (an
// https-to-ssh rewrite defeats the guest's gh-over-HTTPS push, I-150).
// It also denies every key that holds a secret (I-211): a header
// carrying a PAT (http.extraHeader, per URL too), a cookie file, an SMTP
// password, a tool's token (github.token, hub.oauthtoken), a proxy
// command, and any key whose name ends in token, pass, password or
// secret, whatever section a tool invents for it. Tool logins travel as
// files, never inside git config (features/secrets.md).
func gitDenied(key string) bool {
	k := strings.ToLower(key)
	section, _, _ := strings.Cut(k, ".")
	name := k[strings.LastIndex(k, ".")+1:]
	for _, suffix := range []string{"token", "pass", "password", "passwd", "secret"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	switch name {
	case "extraheader", "cookiefile", "proxy", "gitproxy", "proxyauthmethod":
		return true
	}
	switch section {
	case "credential", "ssh", "url", "gpg", "include", "includeif", "difftool", "mergetool", "safe":
		return true
	case "http", "https":
		switch {
		case strings.HasPrefix(name, "proxyssl"),
			strings.HasPrefix(name, "sslca"), strings.HasPrefix(name, "sslcert"), name == "sslkey":
			return true
		}
	}
	switch k {
	case "core.sshcommand", "core.hookspath", "core.askpass", "init.templatedir",
		"user.signingkey", "commit.gpgsign", "tag.gpgsign", "push.gpgsign", "tag.forcesignannotated",
		"diff.tool", "diff.guitool", "merge.tool", "merge.guitool",
		"core.excludesfile": // travels as its contents instead
		return true
	}
	return false
}

// gitCommandKeys are values whose first word is a program the guest must
// have on its PATH.
var gitCommandKeys = map[string]bool{"core.pager": true, "core.editor": true}

// gitCheck is one value the guest must confirm before it is kept.
type gitCheck struct {
	Key, Kind, Value string // Kind: "path" or "cmd"
}

// gitCarry is what the git part sends.
type gitCarry struct {
	Config []byte // the carried file, already rendered
	Checks []gitCheck
	Ignore []byte // core.excludesFile's contents, or nil
	HasID  bool   // user.name or user.email is in it
}

// buildGitCarry reads the laptop's config for repoDir and applies the
// laptop half of I-195: the denylist, core.excludesFile read as a file,
// and the identity the checkout itself resolves (which a repository's
// own .git/config may set) appended last so it wins.
func buildGitCarry(repoDir, homeDir string) (*gitCarry, error) {
	entries, err := readGitConfig(repoDir)
	if err != nil {
		return nil, err
	}
	gc := &gitCarry{}
	var kept []gitEntry
	for _, e := range entries {
		lk := strings.ToLower(e.Key)
		if lk == "core.excludesfile" && e.HasValue {
			if b, err := readLaptopFile(e.Value, homeDir, repoDir); err == nil {
				gc.Ignore = b
			}
			continue
		}
		if gitDenied(e.Key) {
			continue
		}
		kept = append(kept, e)
		switch {
		case !e.HasValue:
		case gitCommandKeys[lk]:
			gc.Checks = append(gc.Checks, gitCheck{Key: e.Key, Kind: "cmd", Value: e.Value})
		case strings.HasPrefix(e.Value, "/") || strings.HasPrefix(e.Value, "~/"):
			gc.Checks = append(gc.Checks, gitCheck{Key: e.Key, Kind: "path", Value: e.Value})
		}
	}
	for _, k := range []string{"user.name", "user.email"} {
		if v, err := gitCmd(repoDir, "config", k); err == nil && v != "" {
			// One value, the one git resolves in the checkout, in place
			// of every earlier one (the global's and an includeIf's).
			n := kept[:0]
			for _, e := range kept {
				if strings.ToLower(e.Key) != k {
					n = append(n, e)
				}
			}
			kept = append(n, gitEntry{Key: k, Value: v, HasValue: true})
			gc.HasID = true
		}
	}
	if len(kept) == 0 && gc.Ignore == nil {
		return nil, nil
	}
	gc.Config = renderGitConfig(kept)
	return gc, nil
}

// readLaptopFile reads a path as git would resolve it on the laptop (~/
// is the home, a relative path is relative to the checkout), up to 1 MB.
func readLaptopFile(p, homeDir, repoDir string) ([]byte, error) {
	switch {
	case p == "~":
		p = homeDir
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(homeDir, p[2:])
	case !filepath.IsAbs(p):
		p = filepath.Join(repoDir, p)
	}
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("%s: not a small regular file", p)
	}
	return os.ReadFile(p)
}

// renderGitConfig writes entries in git's config syntax, in order, one
// section header per run of keys that share it. Every value is quoted
// and escaped, so leading spaces, `#`, `;`, quotes, backslashes and
// newlines survive; a key with no value stays bare (git's "true").
func renderGitConfig(entries []gitEntry) []byte {
	var b strings.Builder
	b.WriteString("# Carried from your laptop by repose on every run and attach; replaced whole each time.\n# Set a key in ~/.gitconfig instead to override it here.\n")
	header := ""
	for _, e := range entries {
		section, sub, name := splitGitKey(e.Key)
		if section == "" || name == "" {
			continue
		}
		h := "[" + section + "]"
		if sub != "" {
			h = "[" + section + " \"" + escapeGitSubsection(sub) + "\"]"
		}
		if h != header {
			b.WriteString(h + "\n")
			header = h
		}
		if !e.HasValue {
			b.WriteString("\t" + name + "\n")
			continue
		}
		b.WriteString("\t" + name + " = \"" + escapeGitValue(e.Value) + "\"\n")
	}
	return []byte(b.String())
}

// splitGitKey splits "section.sub.section.name": the section is up to the
// first dot, the name after the last, the subsection (dots and all) in
// between.
func splitGitKey(key string) (section, sub, name string) {
	i := strings.Index(key, ".")
	j := strings.LastIndex(key, ".")
	if i < 0 || j == i && i == len(key)-1 {
		return "", "", ""
	}
	section, name = key[:i], key[j+1:]
	if j > i {
		sub = key[i+1 : j]
	}
	return section, sub, name
}

func escapeGitSubsection(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func escapeGitValue(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\b", `\b`).Replace(s)
}

// addGitPart adds the git part to p unless the guest already has this
// exact input. It reports whether it was added.
func addGitPart(p *guestPayload, gc *gitCarry, opts carryOptions) (bool, error) {
	if gc == nil {
		return false, nil
	}
	var checks strings.Builder
	for _, c := range gc.Checks {
		_, _ = fmt.Fprintf(&checks, "%s\x00%s\x00%s\x00", c.Key, c.Kind, c.Value)
	}
	hash := carryHash(gc.Config, []byte(checks.String()), gc.Ignore)
	if opts.unchanged("git", hash) {
		return false, nil
	}
	if err := p.file("git/config", gc.Config); err != nil {
		return false, err
	}
	for i, c := range gc.Checks {
		for ext, v := range map[string]string{"key": c.Key, "kind": c.Kind, "val": c.Value} {
			if err := p.file(fmt.Sprintf("git/check-%03d.%s", i, ext), []byte(v)); err != nil {
				return false, err
			}
		}
	}
	if gc.Ignore != nil {
		if err := p.file("git/ignore", gc.Ignore); err != nil {
			return false, err
		}
	}
	return true, p.part("git", gitPartScript()+setMarker("git", hash))
}

// gitPartScript is the guest half of I-195: drop what would not work
// here (a path the guest lacks, a pager or editor not on its PATH), put
// the file in place by rename, and include it from ~/.gitconfig once,
// first, so the guest's own keys come after it and win. The first time,
// an identity the old credential sync wrote straight into ~/.gitconfig
// (I-150) is removed when it equals the carried one, or it would shadow
// every later change on the laptop.
func gitPartScript() string {
	return `t=$1
f="$t/git/config"
for k in "$t"/git/check-*.key; do
  [ -e "$k" ] || continue
  b=${k%.key}
  key=$(cat "$k"); kind=$(cat "$b.kind"); val=$(cat "$b.val")
  ok=
  case $kind in
    path)
      p=$val
      case $p in "~/"*) p="$HOME/${p#"~/"}" ;; esac
      [ -e "$p" ] && ok=1 ;;
    cmd)
      c=${val%%[[:space:]]*}
      case $c in *=*) ok=1 ;; *) command -v "$c" >/dev/null 2>&1 && ok=1 ;; esac ;;
  esac
  if [ -z "$ok" ]; then
    git config --file "$f" --unset-all "$key" || true
    echo "#dropped git $key"
  fi
done
mkdir -p ~/.config/git
cp "$f" ~/.config/git/repose-carried.new
mv -f ~/.config/git/repose-carried.new ~/.config/git/repose-carried
if [ -f "$t/git/ignore" ]; then
  cp "$t/git/ignore" ~/.config/git/ignore.new
  mv -f ~/.config/git/ignore.new ~/.config/git/ignore
fi
g="$HOME/.gitconfig"
if [ -L "$g" ]; then
  echo '#warn ~/.gitconfig is a link, so your laptop git config was not included; add "[include] path = ` + gitCarried + `" to it.'
elif ! git config --file "$g" --get-all include.path 2>/dev/null | grep -qxF '` + gitCarried + `'; then
  { printf '[include]\n\tpath = ` + gitCarried + `\n'; cat "$g" 2>/dev/null || true; } > "$g.repose-new"
  mv -f "$g.repose-new" "$g"
  for k in user.name user.email; do
    c=$(git config --file ~/.config/git/repose-carried --get "$k" || true)
    o=$(git config --file "$g" --get "$k" || true)
    if [ -n "$o" ] && [ "$o" = "$c" ]; then git config --file "$g" --unset "$k" || true; fi
  done
fi
`
}
