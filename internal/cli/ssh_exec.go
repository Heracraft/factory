package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// sshTarget is what runSSH needs to reach a guest: the args ssh needs
// after "ssh" and before the remote command. In production this is just
// the Host alias from ~/.ssh/repose/config ("todo-app.repose"); tests
// point it at a fake guest with -o overrides instead of the real gateway.
type sshTarget struct {
	Args []string // e.g. []string{"todo-app.repose"}
}

func hostTarget(slug string) sshTarget { return sshTarget{Args: []string{slug + ".repose"}} }

// runSSH runs one remote command over one SSH connection, returning
// stdout. A non-zero remote exit is an error carrying stderr.
func runSSH(ctx context.Context, t sshTarget, remoteCmd string, stdin io.Reader) ([]byte, error) {
	args := append(append([]string{}, t.Args...), remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("ssh %s: %w: %s", remoteCmd, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// runSSHOK is runSSH for commands whose output is not needed, only
// success (07-cli.md §5.5 step 4: `ssh <slug>.repose true`).
func runSSHOK(ctx context.Context, t sshTarget, remoteCmd string) error {
	_, err := runSSH(ctx, t, remoteCmd, nil)
	return err
}

// execReplaceSSH is the foreground, process-replacing ssh of `repose run`
// step 8 and `repose attach`/`repose open`: the CLI process becomes ssh so
// signals and the terminal behave exactly like plain ssh (07-cli.md §5.5,
// checklist "the CLI process is replaced").
func execReplaceSSH(t sshTarget, extraArgs []string, remoteCmd string) error {
	args := append(append([]string{}, extraArgs...), t.Args...)
	if remoteCmd != "" {
		args = append(args, remoteCmd)
	}
	return sysExec("ssh", args)
}
