package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// unixEpoch is the mtime carried files are packed with, so the payload's
// bytes, like the hashes, depend on content only.
var unixEpoch = time.Unix(0, 0)

// The Claude Code part of the carry (I-196): the user's own Claude
// configuration, never its credentials or its state. Carried:
// ~/.claude/CLAUDE.md, settings.json (merged in the guest, never
// overwritten), skills/, agents/, commands/, output-styles/,
// keybindings.json, and the scripts under ~/.claude that settings.json
// runs (hooks, statusLine). Everything else in ~/.claude stays on the
// laptop: .credentials.json, projects/ (transcripts), history.jsonl,
// todos/, shell-snapshots/, file-history/, paste-cache/, sessions/,
// plugins/, statsig/, and ~/.claude.json.

//go:embed claude_merge.jq
var claudeMergeJQ []byte

// claudeFiles and claudeDirs are the allowlist; nothing outside it is
// ever read.
var (
	claudeFiles = []string{"CLAUDE.md", "keybindings.json"}
	claudeDirs  = []string{"skills", "agents", "commands", "output-styles"}
)

// claudeNeverDirs are state directories a settings.json script path is
// never taken from, whatever settings.json says.
var claudeNeverDirs = map[string]bool{
	"projects": true, "todos": true, "shell-snapshots": true, "file-history": true, "paste-cache": true,
	"sessions": true, "plugins": true, "statsig": true, "ide": true, "debug": true, "logs": true,
}

// claudeDirCap bounds one carried directory; skills/ was 4.3 MB on the dev
// box (the outlier the proposal measured), so this leaves room and still
// keeps a stray dataset out of the payload.
const claudeDirCap = 32 << 20

// claudeFileCap bounds one carried file.
const claudeFileCap = 4 << 20

// claudeItem is one carried piece: a file or a directory's files, keyed by
// the path relative to ~/.claude.
type claudeItem struct {
	Marker string
	Files  map[string]claudeFile // rel path -> content
}

type claudeFile struct {
	Body []byte
	Mode os.FileMode
}

// claudeCarry is what the Claude part sends.
type claudeCarry struct {
	Items []claudeItem
	// Settings is the laptop's settings.json, already checked to be JSON;
	// nil when absent or invalid.
	Settings []byte
	Home     string // the laptop's home, rewritten to /home/dev in the guest
	// Plugins are enabledPlugins ids from a marketplace the guest can
	// reach; Marketplaces maps their names to an `add` source.
	Plugins      []string
	Marketplaces map[string]string
	// Notes are said once, on the laptop: an invalid settings.json, a
	// directory over the cap, a plugin from a laptop directory.
	Notes []string
}

// claudeConfigDir is where the laptop keeps its Claude config:
// $CLAUDE_CONFIG_DIR when set, else ~/.claude.
func claudeConfigDir(homeDir string) string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(homeDir, ".claude")
}

// buildClaudeCarry reads the allowlisted parts of the laptop's Claude
// config. A laptop with no ~/.claude carries nothing.
func buildClaudeCarry(homeDir string) (*claudeCarry, error) {
	dir := claudeConfigDir(homeDir)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, nil
	}
	cc := &claudeCarry{Home: homeDir, Marketplaces: map[string]string{}}
	for _, name := range claudeFiles {
		b, mode, err := readSmallFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		cc.Items = append(cc.Items, claudeItem{Marker: "claude-" + markerName(name), Files: map[string]claudeFile{name: {b, mode}}})
	}
	for _, d := range claudeDirs {
		files, over, err := readClaudeDir(dir, d)
		if err != nil {
			continue
		}
		if over {
			cc.Notes = append(cc.Notes, fmt.Sprintf("~/.claude/%s is over %d MB, so it was not carried.", d, claudeDirCap>>20))
			continue
		}
		if len(files) > 0 {
			cc.Items = append(cc.Items, claudeItem{Marker: "claude-" + d, Files: files})
		}
	}

	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	switch {
	case err != nil:
	case !json.Valid(raw):
		cc.Notes = append(cc.Notes, "Your ~/.claude/settings.json is not valid JSON, so it was not carried; the guest keeps its own.")
	default:
		cc.Settings = raw
		var s map[string]any
		_ = json.Unmarshal(raw, &s)
		scripts := claudeScripts(s, homeDir, dir)
		if len(scripts) > 0 {
			cc.Items = append(cc.Items, claudeItem{Marker: "claude-scripts", Files: scripts})
		}
		cc.Plugins, cc.Marketplaces, cc.Notes = claudePlugins(s, cc.Notes)
	}
	if len(cc.Items) == 0 && cc.Settings == nil {
		return nil, nil
	}
	return cc, nil
}

// markerName turns a relative path into a marker file name.
func markerName(rel string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(strings.ToLower(rel))
}

func readSmallFile(p string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(p)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > claudeFileCap {
		return nil, 0, fmt.Errorf("%s: not a regular file under the cap", p)
	}
	b, err := os.ReadFile(p)
	return b, info.Mode().Perm(), err
}

// readClaudeDir reads the regular files under dir/d (symlinks and
// anything else are left behind: a link to a laptop path means nothing in
// the guest). over reports the directory past claudeDirCap.
func readClaudeDir(dir, d string) (files map[string]claudeFile, over bool, err error) {
	root := filepath.Join(dir, d)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return nil, false, fmt.Errorf("no %s", d)
	}
	files = map[string]claudeFile{}
	var total int64
	err = filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if de.IsDir() {
			if de.Name() == ".git" || de.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !de.Type().IsRegular() || de.Name() == ".credentials.json" {
			return nil
		}
		info, err := de.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		if total > claudeDirCap {
			over = true
			return filepath.SkipAll
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = claudeFile{b, info.Mode().Perm()}
		return nil
	})
	return files, over, err
}

// claudeScripts finds the files under the laptop's ~/.claude that a hook
// or the statusLine runs, by the words of each command that name a path
// there (~/, $HOME/, ${HOME}/ or the home itself), and reads them.
func claudeScripts(s map[string]any, homeDir, dir string) map[string]claudeFile {
	out := map[string]claudeFile{}
	for _, cmd := range claudeCommands(s) {
		for _, w := range strings.Fields(cmd) {
			w = strings.Trim(w, `"'`)
			for _, pre := range []string{"~/", "$HOME/", "${HOME}/"} {
				if strings.HasPrefix(w, pre) {
					w = filepath.Join(homeDir, w[len(pre):])
				}
			}
			if !filepath.IsAbs(w) {
				continue
			}
			rel, err := filepath.Rel(dir, filepath.Clean(w))
			if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
				continue
			}
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if claudeNeverDirs[parts[0]] || filepath.Base(rel) == ".credentials.json" || rel == "settings.json" {
				continue
			}
			b, mode, err := readSmallFile(filepath.Join(dir, rel))
			if err != nil {
				continue
			}
			out[filepath.ToSlash(rel)] = claudeFile{b, mode}
		}
	}
	return out
}

// claudeCommands lists every hook and statusLine command in a settings
// object, the same shapes claude_merge.jq walks.
func claudeCommands(s map[string]any) []string {
	var cmds []string
	if hooks, ok := s["hooks"].(map[string]any); ok {
		for _, groups := range hooks {
			gs, _ := groups.([]any)
			for _, g := range gs {
				gm, _ := g.(map[string]any)
				hs, _ := gm["hooks"].([]any)
				for _, h := range hs {
					hm, _ := h.(map[string]any)
					if c, ok := hm["command"].(string); ok {
						cmds = append(cmds, c)
					}
				}
			}
		}
	}
	if sl, ok := s["statusLine"].(map[string]any); ok {
		if c, ok := sl["command"].(string); ok {
			cmds = append(cmds, c)
		}
	}
	sort.Strings(cmds)
	return cmds
}

// claudePlugins reads enabledPlugins and extraKnownMarketplaces. Claude
// Code records only names there and does not reinstall on a fresh
// machine, so the guest installs what is missing. A marketplace that is
// a directory on the laptop cannot be reached from the guest; its
// plugins are named once instead.
func claudePlugins(s map[string]any, notes []string) (plugins []string, markets map[string]string, _ []string) {
	markets = map[string]string{}
	local := map[string]bool{}
	if km, ok := s["extraKnownMarketplaces"].(map[string]any); ok {
		for name, v := range km {
			vm, _ := v.(map[string]any)
			src, _ := vm["source"].(map[string]any)
			kind, _ := src["source"].(string)
			switch kind {
			case "github":
				if r, ok := src["repo"].(string); ok {
					markets[name] = r
				}
			case "git", "url":
				if u, ok := src["url"].(string); ok {
					markets[name] = u
				}
			default:
				local[name] = true
			}
		}
	}
	if ep, ok := s["enabledPlugins"].(map[string]any); ok {
		for id, on := range ep {
			if b, ok := on.(bool); !ok || !b {
				continue
			}
			_, m, ok := strings.Cut(id, "@")
			if !ok {
				continue
			}
			if local[m] {
				notes = append(notes, fmt.Sprintf("Plugin %s comes from a marketplace on your laptop, so the guest cannot install it.", id))
				continue
			}
			plugins = append(plugins, id)
		}
	}
	sort.Strings(plugins)
	return plugins, markets, notes
}

// addClaudeParts adds one part per carried item that changed, then the
// settings merge, then the plugin install. It returns the labels added.
func addClaudeParts(p *guestPayload, cc *claudeCarry, opts carryOptions) ([]string, error) {
	if cc == nil {
		return nil, nil
	}
	var sent []string
	for i, it := range cc.Items {
		hash := claudeItemHash(it)
		if opts.unchanged(it.Marker, hash) {
			continue
		}
		base := fmt.Sprintf("claude/i%d", i)
		for _, rel := range sortedFileKeys(it.Files) {
			f := it.Files[rel]
			if err := p.fileMeta(base+"/"+rel, f.Body, int64(f.Mode), unixEpoch); err != nil {
				return nil, err
			}
		}
		// Onto what the guest has: a skill made in the guest stays.
		script := fmt.Sprintf("mkdir -p ~/.claude\ncp -R \"$1/%s/.\" ~/.claude/\n%s", base, setMarker(it.Marker, hash))
		if err := p.part("Claude "+strings.TrimPrefix(it.Marker, "claude-"), script); err != nil {
			return nil, err
		}
		sent = append(sent, it.Marker)
	}
	if cc.Settings != nil {
		hash := carryHash(cc.Settings, []byte(cc.Home), claudeMergeJQ)
		if !opts.unchanged("claude-settings", hash) {
			for name, b := range map[string][]byte{"claude/settings.json": cc.Settings, "claude/home": []byte(cc.Home), "claude/merge.jq": claudeMergeJQ} {
				if err := p.file(name, b); err != nil {
					return nil, err
				}
			}
			if err := p.part("Claude settings", claudeSettingsScript()+setMarker("claude-settings", hash)); err != nil {
				return nil, err
			}
			sent = append(sent, "claude-settings")
		}
	}
	if len(cc.Plugins) > 0 {
		list, _ := json.Marshal(map[string]any{"plugins": cc.Plugins, "marketplaces": cc.Marketplaces})
		hash := carryHash(list)
		if !opts.unchanged("claude-plugins", hash) {
			if err := p.file("claude/plugins.json", list); err != nil {
				return nil, err
			}
			if err := p.file("claude/plugins.sh", []byte(claudePluginsScript(hash))); err != nil {
				return nil, err
			}
			if err := p.part("Claude plugins", `mkdir -p ~/.repose
cp "$1/claude/plugins.json" ~/.repose/claude-plugins.json
cp "$1/claude/plugins.sh" ~/.repose/claude-plugins.sh
setsid -f sh ~/.repose/claude-plugins.sh </dev/null >/dev/null 2>&1
`); err != nil {
				return nil, err
			}
			sent = append(sent, "claude-plugins")
		}
	}
	return sent, nil
}

func claudeItemHash(it claudeItem) string {
	var parts [][]byte
	for _, rel := range sortedFileKeys(it.Files) {
		f := it.Files[rel]
		parts = append(parts, []byte(rel), []byte(fmt.Sprint(f.Mode)), f.Body)
	}
	return carryHash(parts...)
}

func sortedFileKeys(m map[string]claudeFile) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// claudeSettingsScript is the guest half of the merge: rewrite the
// laptop's home, list the commands the result would run, drop the ones
// the guest cannot run (named once), merge, check the result with
// `jq empty`, keep the previous file as settings.json.repose-prev and
// rename the new one into place. An invalid guest file is left alone.
func claudeSettingsScript() string {
	return `t=$1
c="$t/claude"
s="$HOME/.claude/settings.json"
mkdir -p "$HOME/.claude"
echo '{}' > "$t/empty.json"
g="$t/empty.json"
if [ -s "$s" ]; then
  if ! jq empty "$s" 2>/dev/null; then
    echo "#warn The guest's ~/.claude/settings.json is not valid JSON, so your laptop's settings were not merged into it."
    exit 0
  fi
  g="$s"
fi
pf=${REPOSE_CLAUDE_PLATFORM:-/etc/repose/claude-settings.json}
[ -r "$pf" ] || pf="$t/empty.json"
: > "$t/missing"
q() {
  qm=$1 qh=$2 qg=$3 ql=$4
  shift 4
  jq -n "$@" --arg mode "$qm" --arg home "$qh" --arg dest "$HOME" --slurpfile g "$qg" --slurpfile l "$ql" --slurpfile p "$pf" --rawfile missing "$t/missing" -f "$c/merge.jq"
}
q rewrite "$(cat "$c/home")" "$g" "$c/settings.json" > "$t/laptop.json"
cmd_ok() {
  set -f
  first=1
  for w in $1; do
    w=${w#\"}; w=${w%\"}; w=${w#\'}; w=${w%\'}
    case $w in
      "~/"*) w="$HOME/${w#"~/"}" ;;
      '$HOME/'*) w="$HOME/${w#'$HOME/'}" ;;
      '${HOME}/'*) w="$HOME/${w#'${HOME}/'}" ;;
    esac
    case $w in *'$'*|*'` + "`" + `'*) first=; continue ;; esac
    if [ -n "$first" ]; then
      case $w in *=*) continue ;; esac
      first=
      case $w in
        /*) [ -x "$w" ] || { set +f; return 1; } ;;
        *) command -v "$w" >/dev/null 2>&1 || { set +f; return 1; } ;;
      esac
    else
      case $w in /*) [ -e "$w" ] || { set +f; return 1; } ;; esac
    fi
  done
  set +f
  return 0
}
q commands "" "$g" "$t/laptop.json" -r > "$t/commands"
while IFS= read -r b; do
  [ -n "$b" ] || continue
  cmd=$(printf '%s' "$b" | base64 -d)
  if ! cmd_ok "$cmd"; then
    printf '%s\n' "$b" >> "$t/missing"
    printf '#dropped Claude hook "%s"\n' "$(printf '%s' "$cmd" | tr '\n' ' ' | cut -c1-100)"
  fi
done < "$t/commands"
q merge "" "$g" "$t/laptop.json" > "$t/merged.json"
jq empty "$t/merged.json"
if [ -f "$s" ] && cmp -s "$t/merged.json" "$s"; then exit 0; fi
cp "$t/merged.json" "$s.tmp"
chmod 600 "$s.tmp"
jq empty "$s.tmp"
if [ -f "$s" ]; then cp -p "$s" "$s.repose-prev"; fi
mv -f "$s.tmp" "$s"
`
}

// claudePluginsScript installs, in the background, the marketplace
// plugins the laptop has enabled and the guest lacks, with Claude Code's
// own commands (checked against 2.1.278, the base's version on
// 2026-09-23: `claude plugin list --json`, `claude plugin marketplace
// list --json`, `claude plugin marketplace add <source>`, `claude plugin
// install <name>@<marketplace>`). It says what happened in tmux and
// writes its marker only when every install worked, so a failure is
// tried again on the next carry.
func claudePluginsScript(hash string) string {
	return `f="$HOME/.repose/claude-plugins.json"
command -v claude >/dev/null 2>&1 || exit 0
installed=$(claude plugin list --json 2>/dev/null | jq -r '.[].id' 2>/dev/null || true)
known=$(claude plugin marketplace list --json 2>/dev/null | jq -r '.[].name' 2>/dev/null || true)
ok=""
bad=""
for id in $(jq -r '.plugins[]' "$f"); do
  if printf '%s\n' "$installed" | grep -qxF "$id"; then continue; fi
  m=${id#*@}
  if ! printf '%s\n' "$known" | grep -qxF "$m"; then
    src=$(jq -r --arg m "$m" '.marketplaces[$m] // empty' "$f")
    if [ -z "$src" ] && [ "$m" = claude-plugins-official ]; then src=anthropics/claude-plugins-official; fi
    if [ -n "$src" ] && claude plugin marketplace add "$src" >/dev/null 2>&1; then known="$known
$m"; fi
  fi
  if claude plugin install "$id" >/dev/null 2>&1; then ok="$ok $id"; else bad="$bad $id"; fi
done
msg=""
[ -n "$ok" ] && msg="Installed Claude plugins:$ok."
[ -n "$bad" ] && msg="$msg Could not install:$bad (claude plugin install in the guest says why)."
if [ -n "$msg" ] && tmux list-sessions >/dev/null 2>&1; then
  s=$(tmux list-sessions -F '#{session_name}' | head -n 1)
  tmux display-message -d 6000 -t "=$s:" "$msg" 2>/dev/null || true
fi
if [ -z "$bad" ]; then
` + setMarker("claude-plugins", hash) + `fi
`
}
