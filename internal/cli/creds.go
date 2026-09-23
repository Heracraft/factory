package cli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// credRow is one row of docs/interfaces/guest-conventions.md "Credentials
// the CLI syncs into the guest". The laptop and guest paths are identical
// relative to $HOME on both sides.
type credRow struct {
	Label string
	Rel   string
	Mode  os.FileMode
}

var credRows = []credRow{
	{Label: "gh", Rel: filepath.Join(".config", "gh", "hosts.yml"), Mode: 0o600},
	{Label: "codex", Rel: filepath.Join(".codex", "auth.json"), Mode: 0o600},
	{Label: "opencode", Rel: filepath.Join(".local", "share", "opencode", "auth.json"), Mode: 0o600},
}

// credSyncOptions says what the git side of the credential sync should
// set up in the guest.
type credSyncOptions struct {
	// RemoteURL is the project's normalised remote. When it is on
	// github.com and gh's login travelled, the guest's git is told to
	// reach github over HTTPS with gh as the credential helper, so an
	// agent's `git push` works without the laptop's SSH keys (I-150).
	RemoteURL string
	// ghToken returns the laptop's gh token when hosts.yml does not hold
	// one (gh 2.40+ keeps it in the system keyring). Nil means `gh auth
	// token`.
	ghToken func() string
	// Kept is told each login left alone because the guest's copy is
	// newer (a login done inside the guest).
	Kept func(label string)
}

// syncCredentials implements 07-cli.md §5.5 step 6, in one ssh: never
// ~/.claude/.credentials.json, ~/.gemini/oauth_creds.json or any SSH
// private key, however they are named on the laptop — credRows above is
// an allowlist, so nothing outside it ever travels. homeDir is the
// laptop's $HOME; repoDir is the project checkout, whose git config
// supplies user.name/user.email. It returns the labels copied, in table
// order, for the one-line "Credentials: gh, opencode" print. It runs
// before the git steps of the sync so the guest's git already knows who
// the user is and how to reach github when the checkout lands.
func syncCredentials(ctx context.Context, t sshTarget, homeDir, repoDir string, opts credSyncOptions) ([]string, error) {
	var copied []string
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var script strings.Builder
	script.WriteString("set -e\nt=$(mktemp -d)\ntrap 'rm -rf \"$t\"' EXIT\ntar -x -C \"$t\"\n")

	ghCopied := false
	for i, row := range credRows {
		local := filepath.Join(homeDir, row.Rel)
		b, err := os.ReadFile(local)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", local, err)
		}
		if row.Label == "gh" {
			b = ghHostsWithToken(b, opts.ghToken)
			ghCopied = true
		}
		name := fmt.Sprintf("c%d", i)
		if err := tarAddBytes(tw, name, b); err != nil {
			return nil, err
		}
		// features/secrets.md: never overwrite a guest file newer than the
		// laptop's (a login done inside the guest would be clobbered);
		// mtime decides, and the copy takes the laptop's mtime so the
		// next run compares like with like.
		mtime := int64(0)
		if info, err := os.Stat(local); err == nil {
			mtime = info.ModTime().Unix()
		}
		guestPath := "~/" + filepath.ToSlash(row.Rel)
		_, _ = fmt.Fprintf(&script, "d=%s\nif [ -e \"$d\" ] && [ \"$(stat -c %%Y \"$d\")\" -gt %d ]; then echo '#kept %s'; else mkdir -p %s && install -m %o \"$t/%s\" \"$d\" && touch -d @%d \"$d\"; fi\n",
			guestPath, mtime, row.Label, filepath.ToSlash(filepath.Dir(guestPath)), row.Mode.Perm(), name, mtime)
		copied = append(copied, row.Label)
	}

	name, _ := gitCmd(repoDir, "config", "user.name")
	email, _ := gitCmd(repoDir, "config", "user.email")
	if name != "" || email != "" {
		// Through files in the payload, not the command line: the values
		// are the user's and do not belong in a process listing.
		if err := tarAddBytes(tw, "git-name", []byte(name)); err != nil {
			return nil, err
		}
		if err := tarAddBytes(tw, "git-email", []byte(email)); err != nil {
			return nil, err
		}
		script.WriteString("git config --global user.name \"$(cat \"$t/git-name\")\"\n")
		script.WriteString("git config --global user.email \"$(cat \"$t/git-email\")\"\n")
		copied = append(copied, "git")
	}
	if ghCopied && remoteHost(opts.RemoteURL) == "github.com" {
		script.WriteString("git config --global url.https://github.com/.insteadOf git@github.com:\n")
		script.WriteString("git config --global --replace-all credential.https://github.com.helper '!gh auth git-credential'\n")
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if len(copied) == 0 {
		return nil, nil
	}
	out, err := runSSH(ctx, t, script.String(), &buf)
	if err != nil {
		return nil, stepFailed("copy your tool logins to the guest", err, "")
	}
	for _, l := range strings.Split(string(out), "\n") {
		if label, ok := strings.CutPrefix(strings.TrimSpace(l), "#kept "); ok {
			for i, c := range copied {
				if c == label {
					copied = append(copied[:i], copied[i+1:]...)
					break
				}
			}
			if opts.Kept != nil {
				opts.Kept(label)
			}
		}
	}
	return copied, nil
}

// remoteHost is the host part of a normalised remote ("github.com/a/b").
func remoteHost(remote string) string {
	h, _, _ := strings.Cut(strings.TrimSpace(remote), "/")
	return h
}

// ghHostsWithToken returns hosts.yml as it should land in the guest: as
// is when it already carries github.com's oauth_token, else with the
// token gh keeps in the laptop's keyring written under github.com. It is
// the same login the laptop has, in the file the guest's gh reads (the
// guest has no keyring); the second of the three homes CLAUDE.md allows.
func ghHostsWithToken(hosts []byte, token func() string) []byte {
	s := string(hosts)
	lines := strings.Split(s, "\n")
	ghLine := -1
	for i, l := range lines {
		if strings.TrimRight(l, " ") == "github.com:" {
			ghLine = i
			continue
		}
		if ghLine >= 0 && i > ghLine {
			if l != "" && !strings.HasPrefix(l, " ") {
				break // the next host's block
			}
			if strings.HasPrefix(strings.TrimSpace(l), "oauth_token:") {
				return hosts
			}
		}
	}
	if ghLine < 0 {
		return hosts
	}
	if token == nil {
		token = ghAuthToken
	}
	tok := token()
	if tok == "" {
		return hosts
	}
	out := append([]string{}, lines[:ghLine+1]...)
	out = append(out, "    oauth_token: "+tok)
	out = append(out, lines[ghLine+1:]...)
	return []byte(strings.Join(out, "\n"))
}

func ghAuthToken() string {
	out, err := exec.Command("gh", "auth", "token", "--hostname", "github.com").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
