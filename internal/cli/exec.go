package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// `repose exec` and `repose ssh` (DECISIONS I-275). exec runs one command
// in the checkout on the machine, docker exec style: no terminal and no
// stdin unless asked (-i, -t), output streamed, and once the command runs
// the exit code is its own. It sees the environment an agent sees: the
// machine's /etc/profile.d/repose.sh (project variables and named
// secrets) and the checkout's dev environment, loaded by the same
// /etc/repose/devshell.sh the agent wrappers source (I-259). ssh opens an
// interactive login shell in the checkout, outside tmux.

// ExecOptions are `repose exec`'s arguments.
type ExecOptions struct {
	ProjectArg  string
	Command     []string
	Interactive bool // -i: pass this terminal's stdin
	TTY         bool // -t: allocate a terminal on the machine
}

// execDevshell is where a base with I-275 keeps the agent wrappers'
// dev environment loader; an older base has no such file and exec falls
// back to what an interactive shell does, `direnv export` for an allowed
// .envrc.
const execDevshell = "/etc/repose/devshell.sh"

// execScript is the remote command line: cd into the checkout (home, with
// a note on stderr, when it is not there yet), load the environment, then
// exec the command, each argument quoted so the guest's shell passes it
// through unchanged, as docker exec does. A shell pipeline is `sh -c`'s job.
func execScript(slug string, argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = shQuote(a)
	}
	name := argv[0]
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return fmt.Sprintf(`cd ~/%[1]s 2>/dev/null || { echo "repose: ~/%[1]s does not exist on the machine yet; running in ~" >&2; cd ~; }
[ -r /etc/profile.d/repose.sh ] && . /etc/profile.d/repose.sh
if [ -r %[2]s ]; then . %[2]s; _repose_devshell %[3]s; unset -f _repose_devshell
elif command -v direnv >/dev/null 2>&1; then eval "$(direnv export bash 2>/dev/null)"; fi
exec %[4]s`, slug, execDevshell, shQuote(name), strings.Join(quoted, " "))
}

// execSSHArgs are ssh's arguments for opts on target.
func execSSHArgs(t sshTarget, opts ExecOptions, remote string) []string {
	var args []string
	if opts.TTY {
		// -tt: a terminal even when this side's stdin is not one, as
		// docker exec -t gives one regardless.
		args = append(args, "-tt")
	}
	args = append(args, t.Args...)
	return append(args, remote)
}

func newExecCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var opts ExecOptions
	cmd := &cobra.Command{
		Use:   "exec [PROJECT] -- COMMAND [ARG...]",
		Short: "Run one command in the checkout on the machine",
		Long: "Runs COMMAND in the checkout on PROJECT's machine (this checkout's project, by default), with\n" +
			"the environment an agent there has: the project's secrets and its dev shell. Output streams\n" +
			"back; the exit code is the command's. Everything after -- is the command, passed through\n" +
			"as separate words; for a pipeline use sh -c '...'.\n\n" +
			"Without -i the command gets no input, and without -t no terminal; -it gives both, for\n" +
			"something interactive such as a REPL.",
		Example: "  repose exec -- npm test\n  repose exec todo-app -- git log --oneline -5\n  repose exec -it -- psql",
		Args: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			if dash < 0 {
				return cobraUsageError{fmt.Errorf("put the command after --: repose exec [PROJECT] -- COMMAND")}
			}
			if dash > 1 {
				return cobraUsageError{fmt.Errorf("%s takes at most one PROJECT before --, got: %s", cmd.CommandPath(), strings.Join(args[:dash], " "))}
			}
			if len(args) == dash {
				return cobraUsageError{fmt.Errorf("no command after --: repose exec [PROJECT] -- COMMAND")}
			}
			return nil
		},
		ValidArgsFunction: completeProject(env),
		RunE: func(cmd *cobra.Command, args []string) error {
			dash := cmd.ArgsLenAtDash()
			project, err := projectFrom(args[:dash], g)
			if err != nil {
				return err
			}
			opts.ProjectArg = project
			opts.Command = args[dash:]
			e, err := env()
			if err != nil {
				return err
			}
			return ExecCmd(cmd.Context(), e, opts, os.Stdin)
		},
	}
	cmd.Flags().BoolVarP(&opts.Interactive, "interactive", "i", false, "pass this terminal's input to the command")
	cmd.Flags().BoolVarP(&opts.TTY, "tty", "t", false, "give the command a terminal (with -i, for interactive programs)")
	return cmd
}

// ExecCmd implements `repose exec`. Before the command runs, a failure is
// one of repose's exit codes with a message; after, the exit code is the
// command's (ssh's own failure is 255).
func ExecCmd(ctx context.Context, e *Env, opts ExecOptions, stdin io.Reader) error {
	project, err := requireRunningProject(ctx, e, opts.ProjectArg)
	if err != nil {
		return err
	}
	target, err := connect(ctx, e, project)
	if err != nil {
		return err
	}
	c := exec.CommandContext(ctx, "ssh", execSSHArgs(target, opts, execScript(project.Slug, opts.Command))...)
	c.Stdout, c.Stderr = e.Out, e.ErrOut
	if opts.Interactive || opts.TTY {
		c.Stdin = stdin
	}
	c.WaitDelay = sshWaitDelay
	err = c.Run()
	if err != nil && errors.Is(err, exec.ErrWaitDelay) && c.ProcessState != nil && c.ProcessState.Success() {
		err = nil
	}
	if err == nil {
		return nil
	}
	var xe *exec.ExitError
	if errors.As(err, &xe) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return silent(xe.ExitCode())
	}
	return exitf(ExitGeneric, "Could not run ssh: %v. repose exec needs the OpenSSH client on your PATH.", err)
}

// sshShellScript is `repose ssh`'s remote command: the user's login shell,
// interactive, in the checkout (home when there is none yet).
func sshShellScript(slug string) string {
	return fmt.Sprintf(`cd ~/%[1]s 2>/dev/null || cd ~; exec "${SHELL:-bash}" -l`, slug)
}

func newSSHCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "ssh [PROJECT]",
		Short: "Open a shell on the machine, in the checkout (outside tmux)",
		Long: "Opens an interactive login shell in the checkout on PROJECT's machine (this checkout's\n" +
			"project, by default), outside the tmux session: exit ends it. `repose attach` opens the tmux\n" +
			"session instead, and `repose exec PROJECT -- COMMAND` runs one command.",
		Args:              projectArgs,
		ValidArgsFunction: completeProject(env),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFrom(args, g)
			if err != nil {
				return err
			}
			e, err := env()
			if err != nil {
				return err
			}
			return SSHCmd(cmd.Context(), e, project)
		},
	}
}

// SSHCmd implements `repose ssh`: the certificate and config as for
// attach, then ssh replaces this process, so its exit code is ssh's.
func SSHCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireRunningProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	target, err := connect(ctx, e, project)
	if err != nil {
		return err
	}
	return execReplaceSSH(target, []string{"-t"}, sshShellScript(project.Slug))
}
