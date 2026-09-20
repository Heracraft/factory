package cli

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// credRow is one row of docs/interfaces/guest-conventions.md "Credentials
// the CLI syncs into the guest". The laptop and guest paths are identical
// relative to $HOME on both sides, so a single-file tar with that
// relative name extracts to the right place on either end.
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

// Names the CLI never copies, however they are named on the laptop
// (07-cli.md §5.5 step 6 and its checklist).
var neverSyncedNames = []string{".claude/.credentials.json", ".gemini/oauth_creds.json", ".ssh/id_ed25519", ".ssh/id_rsa"}

// syncCredentials implements 07-cli.md §5.5 step 6. homeDir is the
// laptop's $HOME; repoDir is the project checkout, whose git config
// supplies user.name/user.email (falling back to the global config the
// normal way `git config` does). It returns the labels copied, in table
// order, for the one-line "Credentials: gh, opencode" print.
func syncCredentials(ctx context.Context, t sshTarget, homeDir, repoDir string) ([]string, error) {
	var copied []string
	for _, row := range credRows {
		local := filepath.Join(homeDir, row.Rel)
		if _, err := os.Stat(local); err != nil {
			continue
		}
		b, err := os.ReadFile(local)
		if err != nil {
			return copied, fmt.Errorf("reading %s: %w", local, err)
		}
		if err := copyOneFile(ctx, t, row.Rel, b, row.Mode); err != nil {
			return copied, fmt.Errorf("copying %s: %w", row.Label, err)
		}
		copied = append(copied, row.Label)
	}

	name, _ := gitCmd(repoDir, "config", "user.name")
	email, _ := gitCmd(repoDir, "config", "user.email")
	if name != "" || email != "" {
		content := fmt.Sprintf("[user]\n\tname = %s\n\temail = %s\n", name, email)
		if err := copyOneFile(ctx, t, ".gitconfig", []byte(content), 0o644); err != nil {
			return copied, fmt.Errorf("copying git identity: %w", err)
		}
		copied = append(copied, "git")
	}
	return copied, nil
}

func copyOneFile(ctx context.Context, t sshTarget, rel string, content []byte, mode os.FileMode) error {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: rel, Mode: int64(mode.Perm()), Size: int64(len(content))}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if _, err := runSSH(ctx, t, "tar -x -C ~", &buf); err != nil {
		return err
	}
	return runSSHOK(ctx, t, fmt.Sprintf("chmod %o ~/%s", mode.Perm(), rel))
}
