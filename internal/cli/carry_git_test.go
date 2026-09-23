package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The laptop config the checklist names: an includeIf that picks the
// work email, a keychain credential helper, an https-to-ssh rewrite, ssh
// signing, a pager the guest lacks, an alias, and an excludes file.
const laptopGitConfig = `[user]
	name = Lap Top
	email = personal@example.com
[includeIf "gitdir:~/work/"]
	path = ~/.gitconfig-work
[alias]
	st = status -sb
	lg = "log --graph --pretty='%h \"x\" #1 ; ok'"
[credential]
	helper = osxkeychain
[url "git@github.com:"]
	insteadOf = https://github.com/
[gpg]
	format = ssh
[user]
	signingkey = ~/.ssh/id_ed25519.pub
[commit]
	gpgsign = true
[core]
	pager = delta
	editor = vi
	excludesFile = ~/.gitignore_global
	autocrlf = input
[commit]
	template = ~/.gitmessage
[http]
	proxy = http://proxy.corp:3128
[http "https://example.com"]
	sslCAInfo = /etc/corp-ca.pem
[diff]
	tool = kaleidoscope
	colorMoved = default
[pull]
	rebase
`

// laptopHome writes laptopGitConfig into a scratch $HOME with a work
// checkout under ~/work, and returns the home and the checkout.
func laptopHome(t *testing.T) (home, repo string) {
	t.Helper()
	home = withHome(t)
	for name, body := range map[string]string{
		".gitconfig":        laptopGitConfig,
		".gitconfig-work":   "[user]\n\temail = work@corp.example\n",
		".gitignore_global": ".DS_Store\n*.swp\n",
	} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo = filepath.Join(home, "work", "proj")
	mustRun(t, home, "git", "init", "-q", "-b", "main", repo)
	return home, repo
}

func TestGitDenylist(t *testing.T) {
	for key, denied := range map[string]bool{
		"credential.helper": true, "credential.https://github.com.helper": true,
		"url.git@github.com:.insteadof": true, "url.x.pushinsteadof": true,
		"core.sshcommand": true, "ssh.variant": true,
		"user.signingkey": true, "gpg.format": true, "gpg.ssh.program": true, "commit.gpgsign": true, "tag.gpgsign": true,
		"http.proxy": true, "https.proxy": true, "http.https://x.com.proxy": true, "http.sslcainfo": true, "http.sslcert": true, "http.https://x.com.sslcainfo": true,
		"core.hookspath": true, "init.templatedir": true, "safe.directory": true,
		"include.path": true, "includeif.gitdir:~/work/.path": true,
		"diff.tool": true, "merge.tool": true, "difftool.vimdiff.cmd": true, "mergetool.x.cmd": true,
		"core.excludesfile": true,
		"user.name":         false, "user.email": false, "alias.st": false, "core.pager": false, "core.editor": false,
		"pull.rebase": false, "init.defaultbranch": false, "diff.colormoved": false, "http.postbuffer": false, "core.autocrlf": false,
	} {
		if got := gitDenied(key); got != denied {
			t.Errorf("gitDenied(%q) = %v, want %v", key, got, denied)
		}
	}
}

// The rendered file reads back, through git itself, exactly as the list
// it was made from: quoting, escapes, subsections with dots and bare keys.
func TestGitConfigRenderRoundTrip(t *testing.T) {
	entries := []gitEntry{
		{Key: "user.name", Value: `Lap "Top" \ Jr.`, HasValue: true},
		{Key: "alias.lg", Value: "log --pretty='%h' #not a comment ; nor this", HasValue: true},
		{Key: "alias.two", Value: "line one\nline two\ttabbed", HasValue: true},
		{Key: "alias.lead", Value: "  leading and trailing  ", HasValue: true},
		{Key: "pull.rebase", HasValue: false},
		{Key: "branch.feature/x.y.remote", Value: "origin", HasValue: true},
		{Key: `remote.we"ird\name.url`, Value: "https://example.com/a.git", HasValue: true},
		{Key: "alias.empty", Value: "", HasValue: true},
	}
	f := filepath.Join(t.TempDir(), "c")
	if err := os.WriteFile(f, renderGitConfig(entries), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "config", "--file", f, "--list", "-z").Output()
	if err != nil {
		t.Fatalf("git read the rendered file back with an error: %v\n%s", err, renderGitConfig(entries))
	}
	got := parseGitConfigZ(out)
	if len(got) != len(entries) {
		t.Fatalf("read back %d entries, want %d:\n%s", len(got), len(entries), renderGitConfig(entries))
	}
	for i := range entries {
		if got[i] != entries[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], entries[i])
		}
	}
}

// I-195 end to end against the local sshd harness: the alias and the
// includeIf email arrive, no denied key does, a pager the guest lacks and
// a laptop path are dropped and named once, the excludes file travels as
// its contents, and a key set by hand in the guest's ~/.gitconfig wins.
func TestCarryGitConfig(t *testing.T) {
	f := newSyncFixture(t)
	home, repo := laptopHome(t)
	ctx := context.Background()
	// Set by hand in the guest before any carry; must still win after.
	mustRun(t, f.guestHome, "git", "config", "--file", filepath.Join(f.guestHome, ".gitconfig"), "alias.st", "status --short")

	gc, err := buildGitCarry(repo, home)
	if err != nil {
		t.Fatal(err)
	}
	carry := func() *carryOutcome {
		t.Helper()
		out, err := runSSH(ctx, f.target, markerScript(), nil)
		if err != nil {
			t.Fatal(err)
		}
		p := newGuestPayload()
		sent, err := addCarry(p, carryOptions{Git: gc, Markers: parseMarkers(string(out))})
		if err != nil {
			t.Fatal(err)
		}
		o := &carryOutcome{Sent: sent}
		if len(sent) == 0 {
			return o
		}
		res, err := p.run(ctx, f.target)
		if err != nil {
			t.Fatal(err)
		}
		o.parse(string(res))
		return o
	}
	o := carry()
	if len(o.Failed) != 0 || len(o.Warnings) != 0 {
		t.Fatalf("outcome = %+v", o)
	}
	dropped := strings.Join(o.Dropped, ",")
	for _, want := range []string{"git core.pager", "git commit.template"} {
		if !strings.Contains(dropped, want) {
			t.Errorf("dropped = %v, want %q named", o.Dropped, want)
		}
	}
	if strings.Contains(dropped, "core.editor") {
		t.Errorf("vi is on the guest's PATH, yet core.editor was dropped: %v", o.Dropped)
	}

	guestGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = f.guestHome
		cmd.Env = append(filterTestEnv(os.Environ(), "HOME", "XDG_CONFIG_HOME"), "HOME="+f.guestHome)
		var out bytes.Buffer
		cmd.Stdout = &out
		_ = cmd.Run()
		return strings.TrimSpace(out.String())
	}
	list := guestGit("config", "--global", "--includes", "--list", "--show-origin")
	t.Logf("guest git config --global --list --show-origin:\n%s", list)
	if got := guestGit("config", "--global", "--includes", "user.email"); got != "work@corp.example" {
		t.Errorf("guest user.email = %q, want the includeIf one", got)
	}
	if got := guestGit("config", "--global", "--includes", "alias.lg"); got != `log --graph --pretty='%h "x" #1 ; ok'` {
		t.Errorf("guest alias.lg = %q", got)
	}
	if got := guestGit("config", "--global", "--includes", "alias.st"); got != "status --short" {
		t.Errorf("guest alias.st = %q, want the guest's own", got)
	}
	for _, denied := range []string{"credential.", "url.", "gpg.", "signingkey", "gpgsign", "proxy", "sslcainfo", "diff.tool", "includeif", "excludesfile", "core.pager", "commit.template"} {
		if strings.Contains(strings.ToLower(list), denied) {
			t.Errorf("denied or dropped key %q is in the guest's config", denied)
		}
	}
	for _, kept := range []string{"core.editor=vi", "core.autocrlf=input", "pull.rebase", "diff.colormoved=default"} {
		if !strings.Contains(list, kept) {
			t.Errorf("%q missing from the guest's config", kept)
		}
	}
	ignore, err := os.ReadFile(filepath.Join(f.guestHome, ".config", "git", "ignore"))
	if err != nil || string(ignore) != ".DS_Store\n*.swp\n" {
		t.Errorf("excludes file: %q %v", ignore, err)
	}
	cfg, _ := os.ReadFile(filepath.Join(f.guestHome, ".gitconfig"))
	if !strings.HasPrefix(string(cfg), "[include]\n\tpath = "+gitCarried+"\n") {
		t.Errorf("~/.gitconfig does not start with the include:\n%s", cfg)
	}

	// Unchanged: nothing is sent and nothing is named again.
	if o := carry(); len(o.Sent) != 0 || len(o.Dropped) != 0 {
		t.Fatalf("second carry = %+v, want nothing sent", o)
	}
	// A key removed on the laptop disappears from the guest.
	mustRun(t, home, "git", "config", "--global", "--unset", "alias.lg")
	if gc, err = buildGitCarry(repo, home); err != nil {
		t.Fatal(err)
	}
	if o := carry(); len(o.Sent) != 1 {
		t.Fatalf("third carry = %+v, want git sent", o)
	}
	if got := guestGit("config", "--global", "--includes", "alias.lg"); got != "" {
		t.Errorf("alias.lg removed on the laptop is still in the guest: %q", got)
	}
	cfg2, _ := os.ReadFile(filepath.Join(f.guestHome, ".gitconfig"))
	if strings.Count(string(cfg2), gitCarried) != 1 {
		t.Errorf("the include was added twice:\n%s", cfg2)
	}
}

// A guest synced before I-195 has the identity in ~/.gitconfig, written
// by the credential sync; left there it would shadow the carried one
// forever, so the first carry removes it when it is the same value.
func TestCarryGitMovesTheOldIdentity(t *testing.T) {
	f := newSyncFixture(t)
	home, repo := laptopHome(t)
	g := filepath.Join(f.guestHome, ".gitconfig")
	mustRun(t, f.guestHome, "git", "config", "--file", g, "user.name", "Lap Top")
	mustRun(t, f.guestHome, "git", "config", "--file", g, "user.email", "old@example.com")
	gc, err := buildGitCarry(repo, home)
	if err != nil {
		t.Fatal(err)
	}
	p := newGuestPayload()
	if _, err := addCarry(p, carryOptions{Git: gc}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.run(context.Background(), f.target); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(g)
	if strings.Contains(string(b), "name = Lap Top") {
		t.Errorf("the old identical user.name stayed in ~/.gitconfig:\n%s", b)
	}
	if !strings.Contains(string(b), "old@example.com") {
		t.Errorf("an email that differs (set by hand?) was removed:\n%s", b)
	}
}

func filterTestEnv(env []string, drop ...string) []string {
	var out []string
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		keep := true
		for _, d := range drop {
			if k == d {
				keep = false
			}
		}
		if keep {
			out = append(out, kv)
		}
	}
	return out
}
