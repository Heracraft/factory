package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// credRow is one row of docs/interfaces/guest-conventions.md "Credentials
// the CLI syncs into the guest". Rel is the path relative to $HOME in the
// guest, and on the laptop unless Darwin names the macOS one.
type credRow struct {
	Label  string
	Rel    string
	Darwin string // the laptop path on macOS, when it differs
	Mode   os.FileMode
}

var credRows = []credRow{
	{Label: "gh", Rel: filepath.Join(".config", "gh", "hosts.yml"), Mode: 0o600},
	{Label: "codex", Rel: filepath.Join(".codex", "auth.json"), Mode: 0o600},
	{Label: "opencode", Rel: filepath.Join(".local", "share", "opencode", "auth.json"), Mode: 0o600},
	// The Vercel CLI keeps its login in the platform's data directory
	// (DECISIONS I-205's list, proposal item 3).
	{Label: "vercel", Rel: filepath.Join(".local", "share", "com.vercel.cli", "auth.json"), Darwin: filepath.Join("Library", "Application Support", "com.vercel.cli", "auth.json"), Mode: 0o600},
}

// laptopRel is where the row's file is on this laptop.
func (r credRow) laptopRel() string {
	if r.Darwin != "" && goos() == "darwin" {
		return r.Darwin
	}
	return r.Rel
}

// credSyncOptions says what the git side of the credential sync should
// set up in the guest.
type credSyncOptions struct {
	// RemoteURL is the project's normalised remote. Whenever gh's login
	// travelled, the guest's git is told to reach github over HTTPS with
	// gh as the credential helper, so an agent's `git push` works without
	// the laptop's SSH keys (I-150; for every remote since I-247).
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
	copied, _, err := syncCredentialsAndCarry(ctx, t, homeDir, repoDir, opts, carryOptions{})
	return copied, err
}

// syncCredentialsAndCarry is syncCredentials with the carry's parts
// (carry.go) in the same payload, so `run` carries the laptop's config
// without a round trip of its own. A carry part that fails is reported in
// the outcome and never fails the credentials.
func syncCredentialsAndCarry(ctx context.Context, t sshTarget, homeDir, repoDir string, opts credSyncOptions, co carryOptions) ([]string, *carryOutcome, error) {
	c, err := buildCredentialsAndCarry(homeDir, repoDir, opts, co)
	if err != nil {
		return nil, nil, err
	}
	if c.p.empty() {
		// Nothing to write (the identity rides the git part, which is
		// unchanged): no ssh at all.
		return c.copied, &carryOutcome{}, nil
	}
	out, err := c.p.run(ctx, t)
	if err != nil {
		return nil, nil, stepFailed("copy your tool logins to the guest", err, "")
	}
	copied, outcome := c.finish(string(out))
	return copied, outcome, nil
}

// credCarry is the tool logins and the carry, built and not yet sent:
// syncCredentialsAndCarry sends it in an ssh of its own; `run` hands it
// to the sync, whose apply ssh runs it first (DECISIONS I-224).
type credCarry struct {
	p      *guestPayload
	copied []string
	sent   []string
	opts   credSyncOptions
}

// buildCredentialsAndCarry reads the laptop's side of
// syncCredentialsAndCarry into a payload without sending it.
func buildCredentialsAndCarry(homeDir, repoDir string, opts credSyncOptions, co carryOptions) (*credCarry, error) {
	var copied []string
	p := newGuestPayload()

	// The logins' part is built aside first: when the guest's "creds"
	// marker says it already took exactly these bytes, and every file it
	// wrote is still there (the probe's #credsmissing), none of it goes
	// (DECISIONS I-224), and an unchanged `run` sends nothing at all.
	type credFile struct {
		name string
		body []byte
	}
	var files []credFile
	var lines, paths []string
	var labels []string
	hashParts := [][]byte{[]byte("creds-1")}

	ghCopied := false
	for i, row := range credRows {
		local := filepath.Join(homeDir, row.laptopRel())
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
		files = append(files, credFile{name, b})
		// features/secrets.md: never overwrite a guest file newer than the
		// laptop's (a login done inside the guest would be clobbered);
		// mtime decides, and the copy takes the laptop's mtime so the
		// next run compares like with like.
		mtime := int64(0)
		if info, err := os.Stat(local); err == nil {
			mtime = info.ModTime().Unix()
		}
		guestPath := "~/" + filepath.ToSlash(row.Rel)
		lines = append(lines, fmt.Sprintf("d=%s\nif [ -e \"$d\" ] && [ \"$(stat -c %%Y \"$d\")\" -gt %d ]; then echo '#kept %s'; else mkdir -p %s && install -m %o \"$t/%s\" \"$d\" && touch -d @%d \"$d\"; fi",
			guestPath, mtime, row.Label, filepath.ToSlash(filepath.Dir(guestPath)), row.Mode.Perm(), name, mtime))
		paths = append(paths, "$HOME/"+filepath.ToSlash(row.Rel))
		labels = append(labels, row.Label)
		hashParts = append(hashParts, []byte(row.Label), b, []byte(fmt.Sprint(mtime)))
	}

	name, _ := gitCmd(repoDir, "config", "user.name")
	email, _ := gitCmd(repoDir, "config", "user.email")
	var gitID []string
	if co.Git != nil {
		// The identity travels inside the carried git config (I-195),
		// where the checkout's includeIf and its own .git/config have
		// already picked it.
		if co.Git.HasID {
			gitID = []string{"git"}
		}
	} else if name != "" || email != "" {
		// Through files in the payload, not the command line: the values
		// are the user's and do not belong in a process listing.
		files = append(files, credFile{"git-name", []byte(name)}, credFile{"git-email", []byte(email)})
		lines = append(lines, "git config --global user.name \"$(cat \"$t/git-name\")\"", "git config --global user.email \"$(cat \"$t/git-email\")\"")
		labels = append(labels, "git")
		hashParts = append(hashParts, []byte("identity"), []byte(name), []byte(email))
	}
	if ghCopied {
		// Whatever the project's remote: with no agent forwarding (I-247)
		// an SSH URL for github.com (the origin guestd sets, a submodule,
		// a repository cloned in the guest) has no key to use, and gh's
		// login is the one way to github the guest has. Each rewrite is
		// set by value, so a second run, or another insteadOf the user
		// added under the same key, is left as it is.
		lines = append(lines,
			"git config --global --replace-all url.https://github.com/.insteadOf git@github.com: '^git@github\\.com:$'",
			"git config --global --replace-all url.https://github.com/.insteadOf ssh://git@github.com/ '^ssh://git@github\\.com/$'",
			"git config --global --replace-all credential.https://github.com.helper '!gh auth git-credential'")
		hashParts = append(hashParts, []byte("gh-helper-2"))
	}
	if len(lines) > 0 {
		hash := carryHash(hashParts...)
		if co.unchanged(credsMarker, hash) {
			timingf("carry: logins unchanged since the guest took them")
		} else {
			for _, f := range files {
				if err := p.file(f.name, f.body); err != nil {
					return nil, err
				}
			}
			for _, l := range lines {
				p.line(l)
			}
			// The paths the probe checks, then the marker, last: a login
			// part that stopped half way leaves the old marker, or none.
			p.line(fmt.Sprintf("mkdir -p ~/.repose && printf '%%s\\n' %s > %s", strings.Join(quoteAll(paths), " "), credsPathsFile))
			p.line(strings.TrimSuffix(setMarker(credsMarker, hash), "\n"))
			copied = append(copied, labels...)
		}
	}
	// "git" is named when the carried git config holds the identity,
	// whether or not its part travels (I-195), after the logins.
	copied = append(copied, gitID...)
	sent, err := addCarry(p, co)
	if err != nil {
		return nil, err
	}
	return &credCarry{p: p, copied: copied, sent: sent, opts: opts}, nil
}

// credsMarker is the carry marker of the tool logins (carry.go's markers:
// ~/.repose/carry/creds holds the hash of what the guest last took), and
// credsPathsFile lists the files they wrote, which the sync's probe
// checks are all still there (I-224).
const (
	credsMarker    = "creds"
	credsPathsFile = "~/.repose/creds-paths"
)

// quoteAll double-quotes each "$HOME/..." path so the guest's shell
// expands $HOME and nothing else (the rows' paths hold no quote, $, `
// or backslash).
func quoteAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = `"` + p + `"`
	}
	return out
}

// finish reads the guest's reply to the payload: the logins copied (less
// the ones the guest kept as newer) and the carry's outcome.
func (c *credCarry) finish(out string) ([]string, *carryOutcome) {
	copied, opts := append([]string(nil), c.copied...), c.opts
	outcome := &carryOutcome{Sent: c.sent}
	outcome.parse(out)
	var kept []string
	for _, label := range outcome.Kept {
		isCred := false
		for i, c := range copied {
			if c == label {
				copied = append(copied[:i], copied[i+1:]...)
				isCred = true
				break
			}
		}
		if !isCred {
			kept = append(kept, label)
			continue
		}
		if opts.Kept != nil {
			opts.Kept(label)
		}
	}
	outcome.Kept = kept
	return copied, outcome
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
