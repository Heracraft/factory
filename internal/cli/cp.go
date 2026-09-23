package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// `repose cp` (DECISIONS I-201): a thin wrapper over scp with the project
// resolved the way every other command resolves it. `<project>:<path>`
// names a file in that project's guest, `:<path>` one in the current
// project's; a relative guest path is taken from ~/<slug>, the checkout.
//
//	repose cp :logs/x.log .
//	repose cp izma:/tmp/trace.json ./trace.json
//	repose cp -r ./fixtures :test/fixtures

// scpExtraArgs is added to every scp; tests set -O (the classic protocol)
// because the local sshd harness has no SFTP server. A real guest's sshd
// does, so modern scp's SFTP mode is what users get.
var scpExtraArgs []string

// cpSide is one argument of `repose cp`.
type cpSide struct {
	Remote  bool
	Project string // "" is the current project
	Path    string
}

// parseCpSide follows scp's rule: a colon before any slash makes it
// remote; a path that starts with / or . is always local, so ./a:b is a
// file.
func parseCpSide(arg string) cpSide {
	if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, ".") {
		return cpSide{Path: arg}
	}
	i := strings.Index(arg, ":")
	if i < 0 || strings.Contains(arg[:i], "/") {
		return cpSide{Path: arg}
	}
	return cpSide{Remote: true, Project: arg[:i], Path: arg[i+1:]}
}

// guestPath makes a guest path scp understands: relative to the
// checkout unless absolute or ~-based.
func (s cpSide) guestPath(slug string) string {
	p := s.Path
	switch {
	case p == "" || p == ".":
		return slug
	case strings.HasPrefix(p, "/"), p == "~", strings.HasPrefix(p, "~/"):
		return p
	default:
		return slug + "/" + p
	}
}

func newCpCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "cp [-r] SRC DST",
		Short: "Copy files to or from a project's guest (PROJECT:PATH, or :PATH for this checkout's)",
		Long: `Copy files between the laptop and a guest with scp. One side names the
guest: PROJECT:PATH for a project, :PATH for this checkout's. A relative
guest path starts at the project's checkout (~/<slug>).

  repose cp :logs/x.log .
  repose cp izma:/tmp/trace.json .
  repose cp -r ./fixtures :test/fixtures`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return cobraUsageError{errors.New("repose cp takes a source and a destination, one of them PROJECT:PATH or :PATH")}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return CpCmd(cmd.Context(), e, args[0], args[1], recursive, g.project)
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "copy directories")
	return cmd
}

// CpCmd resolves the one guest side, refreshes the certificate the way
// run does, and runs scp over the project's multiplexed connection.
func CpCmd(ctx context.Context, e *Env, srcArg, dstArg string, recursive bool, projectFlag string) error {
	src, dst := parseCpSide(srcArg), parseCpSide(dstArg)
	if src.Remote == dst.Remote {
		return exitf(ExitUsage, "One side of `repose cp` names the guest (PROJECT:PATH, or :PATH for this checkout's project) and the other the laptop.")
	}
	remote := &src
	if dst.Remote {
		remote = &dst
	}
	name := remote.Project
	if name != "" && projectFlag != "" && name != projectFlag {
		return exitf(ExitUsage, "Two projects named: %s and --project %s.", name, projectFlag)
	}
	if name == "" {
		name = projectFlag
	}
	project, err := requireRunningProject(ctx, e, name)
	if err != nil {
		return err
	}
	target, err := connect(ctx, e, project)
	if err != nil {
		return err
	}
	host, opts := scpTarget(target)
	args := append([]string{}, scpExtraArgs...)
	args = append(args, opts...)
	if recursive {
		args = append(args, "-r")
	}
	for _, s := range []cpSide{src, dst} {
		if s.Remote {
			args = append(args, host+":"+s.guestPath(project.Slug))
		} else {
			args = append(args, s.Path)
		}
	}
	cmd := exec.CommandContext(ctx, "scp", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.Out, e.ErrOut
	if err := cmd.Run(); err != nil {
		var xe *exec.ExitError
		if errors.As(err, &xe) {
			return silent(xe.ExitCode())
		}
		return exitf(ExitGeneric, "Could not run scp: %v. repose cp needs the OpenSSH client's scp on your PATH.", err)
	}
	return nil
}

// scpTarget splits an ssh target into scp's options and host: scp spells
// ssh's -p as -P, and takes everything else ssh does.
func scpTarget(t sshTarget) (host string, opts []string) {
	a := t.Args
	if len(a) == 0 {
		return "", nil
	}
	host = a[len(a)-1]
	for i := 0; i < len(a)-1; i++ {
		if a[i] == "-p" && i+1 < len(a)-1 {
			opts = append(opts, "-P", a[i+1])
			i++
			continue
		}
		opts = append(opts, a[i])
	}
	return host, opts
}
