package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/claude-merge/*/want.json")

// runClaudeMerge runs the guest half of the settings merge exactly as the
// carry does (claudeSettingsScript with the embedded jq program), with
// home as the guest's $HOME, and returns its output lines.
func runClaudeMerge(t *testing.T, home string, laptop []byte, laptopHome string) string {
	t.Helper()
	dir := t.TempDir()
	c := filepath.Join(dir, "claude")
	if err := os.MkdirAll(c, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"settings.json": laptop, "home": []byte(laptopHome), "merge.jq": claudeMergeJQ} {
		if err := os.WriteFile(filepath.Join(c, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(dir, "part.sh")
	if err := os.WriteFile(script, []byte(claudeSettingsScript()), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-e", script, dir)
	cmd.Env = append(filterTestEnv(os.Environ(), "HOME"), "HOME="+home, "REPOSE_CLAUDE_PLATFORM="+filepath.Join("testdata", "claude-merge", "platform.json"))
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("merge script: %v\n%s%s", err, out.String(), stderr.String())
	}
	return out.String()
}

// The golden cases: permissions union (union), the laptop-home rewrite
// with hooks the guest cannot run dropped (rewrite), repose-hook entries
// stripped from both sides and the platform's added once, last (hooks),
// and a guest with no settings.json yet (fresh). Each is merged twice;
// the second run must not change a byte.
func TestClaudeSettingsMergeGolden(t *testing.T) {
	cases, err := os.ReadDir(filepath.Join("testdata", "claude-merge"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if !c.IsDir() {
			continue
		}
		t.Run(c.Name(), func(t *testing.T) {
			src := filepath.Join("testdata", "claude-merge", c.Name())
			home := t.TempDir()
			settings := filepath.Join(home, ".claude", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(src, "guest.json")); err == nil {
				if err := os.WriteFile(settings, b, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// What the scripts part carried before the merge runs.
			for _, s := range []string{".claude/statusline.sh", ".claude/hooks/fmt.sh"} {
				p := filepath.Join(home, s)
				_ = os.MkdirAll(filepath.Dir(p), 0o700)
				if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			laptop, err := os.ReadFile(filepath.Join(src, "laptop.json"))
			if err != nil {
				t.Fatal(err)
			}
			lh, _ := os.ReadFile(filepath.Join(src, "home"))
			out := runClaudeMerge(t, home, laptop, strings.TrimSpace(string(lh)))
			first, err := os.ReadFile(settings)
			if err != nil {
				t.Fatal(err)
			}
			// The guest's home is /home/dev; this one is a temporary
			// directory, so the goldens are written with it put back.
			first = bytes.ReplaceAll(first, []byte(home), []byte("/home/dev"))
			out = strings.ReplaceAll(out, home, "/home/dev")
			if !json.Valid(first) {
				t.Fatalf("merge wrote invalid JSON:\n%s", first)
			}
			golden := filepath.Join(src, "want.json")
			if *updateGolden {
				if err := os.WriteFile(golden, first, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(src, "want.out"), []byte(out), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(first, want) {
				t.Errorf("merged settings.json differs from %s:\n%s", golden, first)
			}
			wantOut, _ := os.ReadFile(filepath.Join(src, "want.out"))
			if out != string(wantOut) {
				t.Errorf("merge output = %q, want %q", out, wantOut)
			}
			if _, err := os.Stat(settings + ".repose-prev"); (err == nil) != fileExists(filepath.Join(src, "guest.json")) {
				t.Errorf("settings.json.repose-prev present = %v, want it exactly when there was a guest file", err == nil)
			}

			// Idempotent: the same laptop file again changes nothing.
			runClaudeMerge(t, home, laptop, strings.TrimSpace(string(lh)))
			second, _ := os.ReadFile(settings)
			second = bytes.ReplaceAll(second, []byte(home), []byte("/home/dev"))
			if !bytes.Equal(first, second) {
				t.Errorf("second merge changed the file:\nfirst:\n%s\nsecond:\n%s", first, second)
			}
		})
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// A guest settings.json that is not JSON is left exactly as it is, with
// one warning; nothing half-written appears beside it.
func TestClaudeSettingsMergeLeavesAnInvalidGuestFile(t *testing.T) {
	home := t.TempDir()
	settings := filepath.Join(home, ".claude", "settings.json")
	_ = os.MkdirAll(filepath.Dir(settings), 0o700)
	bad := []byte(`{"model": "opus",`)
	if err := os.WriteFile(settings, bad, 0o600); err != nil {
		t.Fatal(err)
	}
	out := runClaudeMerge(t, home, []byte(`{"model":"sonnet"}`), "/Users/lap")
	if !strings.Contains(out, "#warn The guest's ~/.claude/settings.json is not valid JSON") {
		t.Errorf("output = %q", out)
	}
	if b, _ := os.ReadFile(settings); !bytes.Equal(b, bad) {
		t.Errorf("invalid guest file changed to %q", b)
	}
	for _, leftover := range []string{".tmp", ".repose-prev"} {
		if fileExists(settings + leftover) {
			t.Errorf("%s left behind", leftover)
		}
	}
}

// An invalid laptop settings.json never leaves the laptop, and says so.
func TestClaudeCarrySkipsAnInvalidLaptopFile(t *testing.T) {
	home := t.TempDir()
	d := filepath.Join(home, ".claude")
	_ = os.MkdirAll(d, 0o700)
	if err := os.WriteFile(filepath.Join(d, "settings.json"), []byte(`{nope`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "CLAUDE.md"), []byte("be terse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cc, err := buildClaudeCarry(home)
	if err != nil {
		t.Fatal(err)
	}
	if cc.Settings != nil {
		t.Fatal("an invalid settings.json would be sent")
	}
	if len(cc.Notes) != 1 || !strings.Contains(cc.Notes[0], "not valid JSON") {
		t.Errorf("notes = %v", cc.Notes)
	}
	if len(cc.Items) != 1 || cc.Items[0].Marker != "claude-claude-md" {
		t.Errorf("items = %+v", cc.Items)
	}
}

// claudeLaptopHome is a laptop $HOME with the Claude config I-196 carries
// and, beside it, everything it must never carry.
func claudeLaptopHome(t *testing.T, plugins bool) string {
	t.Helper()
	home := t.TempDir()
	enabled := `"mine@laptop-dir":true`
	if plugins {
		enabled = `"gopls-lsp@claude-plugins-official":true,` + enabled
	}
	files := map[string]string{
		".claude/CLAUDE.md":              "Prefer small commits.\n",
		".claude/keybindings.json":       `{"bindings":[]}`,
		".claude/skills/deploy/SKILL.md": "---\nname: deploy\ndescription: ship it\n---\nRun the deploy.\n",
		".claude/agents/reviewer.md":     "---\nname: reviewer\n---\nReview.\n",
		".claude/commands/fix.md":        "Fix the build.\n",
		".claude/hooks/notify.sh":        "#!/bin/sh\necho done\n",
		".claude/settings.json":          `{"model":"opus","permissions":{"allow":["Bash(go test:*)"]},"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"` + home + `/.claude/hooks/notify.sh"}]}]},"enabledPlugins":{` + enabled + `},"extraKnownMarketplaces":{"laptop-dir":{"source":{"source":"directory","path":"/Users/lap/mkt"}}}}`,
		// Never carried, whatever else happens:
		".claude/.credentials.json":               `{"claudeAiOauth":{"accessToken":"NEVER-CLAUDE-CREDS"}}`,
		".claude/skills/deploy/.credentials.json": `NEVER-NESTED-CREDS`,
		".claude/projects/-home-x/abc.jsonl":      `{"NEVER-TRANSCRIPT":1}`,
		".claude/history.jsonl":                   `{"display":"NEVER-HISTORY"}`,
		".claude/todos/t.json":                    `NEVER-TASK-LIST`,
		".claude/shell-snapshots/s.sh":            `NEVER-SNAPSHOT`,
		".claude/file-history/f":                  `NEVER-FILE-HISTORY`,
		".claude/plugins/cache/x":                 `NEVER-PLUGIN-CACHE`,
		".claude/statsig/s":                       `NEVER-STATSIG`,
		".claude.json":                            `{"oauthAccount":"NEVER-CLAUDE-JSON"}`,
		".ssh/id_ed25519":                         "NEVER-SSH-KEY",
		".gemini/oauth_creds.json":                `NEVER-GEMINI`,
	}
	for rel, body := range files {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o600)
		if strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// I-196 and the secrets rule: the whole stream of a run's credentials ssh
// with every carry part in it holds none of the never-carried files,
// while the config does arrive, the hook script runs from the guest, a
// permission granted in the guest survives the next carry, and the
// platform hooks are there once.
func TestCarryClaudeNeverCarriesSecrets(t *testing.T) {
	f := newSyncFixture(t)
	// No plugin the guest could install: the installer runs claude, which
	// TestClaudePluginsScript covers with a stand-in.
	home := claudeLaptopHome(t, false)
	ctx := context.Background()
	var stream bytes.Buffer
	observePayload = func(script string, tarball []byte) { stream.WriteString(script); stream.Write(tarball) }
	t.Cleanup(func() { observePayload = nil })

	carry := func() *carryOutcome {
		t.Helper()
		cc, err := buildClaudeCarry(home)
		if err != nil {
			t.Fatal(err)
		}
		out, err := runSSH(ctx, f.target, markerScript(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, o, err := syncCredentialsAndCarry(ctx, f.target, home, f.local, credSyncOptions{}, carryOptions{TZ: "UTC", Claude: cc, Markers: parseMarkers(string(out))})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	o := carry()
	if len(o.Failed) != 0 {
		t.Fatalf("outcome = %+v", o)
	}
	for _, never := range []string{"NEVER-CLAUDE-CREDS", "NEVER-NESTED-CREDS", "NEVER-TRANSCRIPT", "NEVER-HISTORY", "NEVER-TASK-LIST", "NEVER-SNAPSHOT", "NEVER-FILE-HISTORY", "NEVER-PLUGIN-CACHE", "NEVER-STATSIG", "NEVER-CLAUDE-JSON", "NEVER-SSH-KEY", "NEVER-GEMINI"} {
		if bytes.Contains(stream.Bytes(), []byte(never)) {
			t.Errorf("%s is in the carry stream", never)
		}
	}
	t.Logf("carry stream: %d bytes, none of the 12 never-carried markers in it; sent %v", stream.Len(), o.Sent)
	for _, rel := range []string{".claude/CLAUDE.md", ".claude/keybindings.json", ".claude/skills/deploy/SKILL.md", ".claude/agents/reviewer.md", ".claude/commands/fix.md", ".claude/hooks/notify.sh"} {
		if !fileExists(filepath.Join(f.guestHome, rel)) {
			t.Errorf("%s did not arrive", rel)
		}
	}
	for _, rel := range []string{".claude/.credentials.json", ".claude/projects", ".claude/history.jsonl", ".claude.json"} {
		if fileExists(filepath.Join(f.guestHome, rel)) {
			t.Errorf("%s is in the guest", rel)
		}
	}
	if info, err := os.Stat(filepath.Join(f.guestHome, ".claude/hooks/notify.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook script not executable in the guest: %v %v", info, err)
	}
	var s map[string]any
	b, _ := os.ReadFile(filepath.Join(f.guestHome, ".claude/settings.json"))
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("guest settings: %v\n%s", err, b)
	}
	if !strings.Contains(string(b), f.guestHome+"/.claude/hooks/notify.sh") || strings.Contains(string(b), home) {
		t.Errorf("hook path not rewritten to the guest's home:\n%s", b)
	}

	// A permission granted inside the guest ("Yes, and don't ask again").
	s["permissions"].(map[string]any)["allow"] = append(s["permissions"].(map[string]any)["allow"].([]any), "Bash(make:*)")
	nb, _ := json.MarshalIndent(s, "", "  ")
	if err := os.WriteFile(filepath.Join(f.guestHome, ".claude/settings.json"), nb, 0o600); err != nil {
		t.Fatal(err)
	}
	// Nothing changed on the laptop: nothing is sent.
	if o := carry(); len(o.Sent) != 0 {
		t.Fatalf("unchanged carry sent %v", o.Sent)
	}
	// The laptop changes its settings: the merge runs, the guest's
	// permission stays.
	sp := filepath.Join(home, ".claude/settings.json")
	lb, _ := os.ReadFile(sp)
	if err := os.WriteFile(sp, bytes.Replace(lb, []byte(`"opus"`), []byte(`"sonnet"`), 1), 0o600); err != nil {
		t.Fatal(err)
	}
	carry()
	b, _ = os.ReadFile(filepath.Join(f.guestHome, ".claude/settings.json"))
	if !strings.Contains(string(b), "Bash(make:*)") || !strings.Contains(string(b), `"sonnet"`) {
		t.Errorf("after the laptop changed:\n%s", b)
	}
	if !fileExists(filepath.Join(f.guestHome, ".claude/settings.json.repose-prev")) {
		t.Error("no settings.json.repose-prev")
	}
}

func TestClaudePluginsFromALaptopDirectoryAreNamed(t *testing.T) {
	cc, err := buildClaudeCarry(claudeLaptopHome(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if len(cc.Plugins) != 1 || cc.Plugins[0] != "gopls-lsp@claude-plugins-official" {
		t.Errorf("plugins = %v", cc.Plugins)
	}
	if len(cc.Notes) != 1 || !strings.Contains(cc.Notes[0], "mine@laptop-dir") {
		t.Errorf("notes = %v", cc.Notes)
	}
}

// The background installer, with a stand-in claude on PATH that records
// its calls: it adds the official marketplace when the guest lacks it,
// installs what is missing, skips what is there, and writes its marker
// only when every install worked.
func TestClaudePluginsScript(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	stub := `#!/bin/sh
echo "$*" >> ` + calls + `
case "$*" in
  "plugin list --json") echo '[{"id":"have@claude-plugins-official"}]' ;;
  "plugin marketplace list --json") echo '[]' ;;
  "plugin install broken@claude-plugins-official") exit 1 ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(plugins ...string) {
		t.Helper()
		list, _ := json.Marshal(map[string]any{"plugins": plugins, "marketplaces": map[string]string{}})
		_ = os.MkdirAll(filepath.Join(home, ".repose"), 0o700)
		if err := os.WriteFile(filepath.Join(home, ".repose", "claude-plugins.json"), list, 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Remove(calls)
		cmd := exec.Command("sh", "-c", claudePluginsScript("h1"))
		cmd.Env = append(filterTestEnv(os.Environ(), "HOME", "PATH", "TMUX"), "HOME="+home, "PATH="+bin+":"+os.Getenv("PATH"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("installer: %v\n%s", err, out)
		}
	}
	marker := filepath.Join(home, ".repose", "carry", "claude-plugins")

	run("have@claude-plugins-official", "new@claude-plugins-official", "broken@claude-plugins-official")
	got, _ := os.ReadFile(calls)
	for _, want := range []string{"plugin marketplace add anthropics/claude-plugins-official", "plugin install new@claude-plugins-official", "plugin install broken@claude-plugins-official"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("calls lack %q:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "install have@") {
		t.Errorf("an installed plugin was installed again:\n%s", got)
	}
	if fileExists(marker) {
		t.Error("marker written although an install failed")
	}
	run("have@claude-plugins-official", "new@claude-plugins-official")
	if b, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(b)) != "h1" {
		t.Errorf("marker after a clean run: %q %v", b, err)
	}
}
