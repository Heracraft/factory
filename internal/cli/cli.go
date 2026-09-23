package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Execute is cmd/repose's entry point. version is the build's -ldflags
// value ("dev" outside a release build). It returns the process exit code
// per docs/interfaces/cli-config.md.
func Execute(version string) int {
	root := newRootCmd(version)
	root.SilenceErrors = true
	root.SilenceUsage = true
	// Ctrl-C cancels the command's context, so a spinner line is cleared
	// and child ssh processes are ended, instead of the process dying
	// mid-line.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Interrupted.")
		return 130
	}
	if usageErr, ok := err.(cobraUsageError); ok {
		_, _ = fmt.Fprintln(os.Stderr, usageErr.Error())
		return ExitUsage
	}
	// cobra's own refusals (an unknown command, a wrong argument count)
	// are usage mistakes too, not command failures.
	if msg := err.Error(); strings.HasPrefix(msg, "unknown command") || strings.HasPrefix(msg, "accepts ") || strings.HasPrefix(msg, "invalid argument") {
		_, _ = fmt.Fprintf(os.Stderr, "%s\nRun `repose --help` for the commands.\n", msg)
		return ExitUsage
	}
	return exitCodeFor(err, os.Stderr)
}

// cobraUsageError marks an error as a plain usage mistake (bad flags,
// wrong arg count) rather than a command failure.
type cobraUsageError struct{ error }

type globalFlags struct {
	command string // the running command's path, set before RunE
	project string
	apiURL  string
	json    bool
	verbose bool
}

func newRootCmd(version string) *cobra.Command {
	g := &globalFlags{}
	root := &cobra.Command{
		Use:           "repose",
		Short:         "repose: persistent remote environments for coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
		// Version makes cobra accept `repose --version` (docs/CHECKLIST.md
		// "Release (M5)"); the template keeps it byte-identical to
		// `repose version`, herakraft suffix included.
		Version: version,
	}
	root.SetVersionTemplate("repose {{.Version}} (herakraft)\n")
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return cobraUsageError{fmt.Errorf("%v (`%s --help` lists its flags)", err, cmd.CommandPath())}
	})
	root.PersistentFlags().StringVar(&g.project, "project", "", "project name or id (or $REPOSE_PROJECT); most commands also take it as their argument")
	root.PersistentFlags().StringVar(&g.apiURL, "api-url", "", "api base url (or $REPOSE_API_URL)")
	root.PersistentFlags().BoolVarP(&g.verbose, "verbose", "v", false, "debug logging to stderr")

	env := func() (*Env, error) {
		e, err := newEnv(g.apiURL, g.json, g.verbose)
		if err != nil {
			return nil, err
		}
		e.Client.HTTP = e.httpClient
		e.Command = g.command
		return e, nil
	}
	root.PersistentPreRun = func(cmd *cobra.Command, args []string) { g.command = cmd.CommandPath() }
	envJSON := func(cmd *cobra.Command) (*Env, error) {
		json, _ := cmd.Flags().GetBool("json")
		g.json = json
		return env()
	}
	_ = root.RegisterFlagCompletionFunc("project", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return projectSlugsForCompletion(env), cobra.ShellCompDirectiveNoFileComp
	})

	root.AddCommand(
		newLoginCmd(),
		newLogoutCmd(env),
		newRunCmd(env, g),
		newAttachCmd(env, g),
		newStartCmd(env, g),
		newStopCmd(env, g),
		newStatusCmd(envJSON, env, g),
		newOpenCmd(env, g),
		newSecretsCmd(env, g),
		newConfigCmd(env, g),
		newSnapshotsCmd(env, g),
		newDestroyCmd(env, g),
		newRestoreCmd(env),
		newLogsCmd(envJSON, env, g),
		newProjectsCmd(envJSON),
		newEventsCmd(envJSON, env, g),
		newNotifyCmd(env),
		newResizeCmd(env, g),
		newVersionCmd(version),
		newCompletionCmd(),
		newMCPCmd(),
		newBrowserCmd(),
	)
	return root
}

// Positional PROJECT (DECISIONS I-155): every command whose object is a
// project takes it as its one argument, docker-style (`repose attach
// izma`), with --project and $REPOSE_PROJECT still working.

// projectArgs is the Args validator for those commands.
func projectArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return cobraUsageError{fmt.Errorf("%s takes at most one PROJECT, got %d arguments: %s", cmd.CommandPath(), len(args), strings.Join(args, " "))}
	}
	return nil
}

// noArgs is cobra.NoArgs as a usage error: v0.1.4 silently ignored a
// stray word (`repose attach projects` attached to the cwd's project).
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return cobraUsageError{fmt.Errorf("%s takes no arguments, got: %s", cmd.CommandPath(), strings.Join(args, " "))}
	}
	return nil
}

// projectFrom picks the project named by the positional argument or
// --project; naming two different ones is a usage error.
func projectFrom(args []string, g *globalFlags) (string, error) {
	if len(args) == 0 {
		return g.project, nil
	}
	if g.project != "" && g.project != args[0] {
		return "", cobraUsageError{fmt.Errorf("%q and --project %q name two projects; pass one", args[0], g.project)}
	}
	return args[0], nil
}

// completeProject completes the one PROJECT argument with the account's
// slugs.
func completeProject(env func() (*Env, error)) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return projectSlugsForCompletion(env), cobra.ShellCompDirectiveNoFileComp
	}
}

// projectSlugsForCompletion asks the api (two seconds at most, a shell is
// waiting) and falls back to the slugs in projects.json.
func projectSlugsForCompletion(env func() (*Env, error)) []string {
	e, err := env()
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	seen := map[string]bool{}
	if projects, err := e.Client.ListProjects(ctx); err == nil {
		for _, p := range projects {
			seen[p.Slug] = true
		}
	} else {
		for _, c := range e.Cache.ByRemote {
			if c.Slug != "" {
				seen[c.Slug] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func newLoginCmd() *cobra.Command {
	var noBrowser, browser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with Logto",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := newEnv("", false, false)
			if err != nil {
				return err
			}
			opts := loginOptions{
				Browser:   browser,
				NoBrowser: noBrowser || os.Getenv("REPOSE_NO_BROWSER") == "1",
				Display:   os.Getenv("DISPLAY"),
				GOOS:      goos(),
				GuestEnv:  os.Getenv("REPOSE") == "1",
			}
			return runLogin(cmd.Context(), e.Dir, e.Cfg, e.httpClient, opts)
		},
	}
	cmd.Flags().BoolVar(&browser, "browser", false, "use the loopback browser flow (PKCE) instead of the device code; needs a Logto application with loopback redirect URIs")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "device-code flow (the default since v0.1.2; kept for scripts)")
	return cmd
}

func newLogoutCmd(env func() (*Env, error)) *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return runLogout(cmd.Context(), e.Dir, e.Cfg, e.httpClient, purge)
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also remove ~/.ssh/repose, ~/.config/repose and the Include line")
	return cmd
}

func newRunCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var opts RunOptions
	cmd := &cobra.Command{
		Use:   "run [PROMPT]",
		Short: "Create/start this checkout's environment, sync it and attach; with PROMPT, start an agent on it",
		Long: "Create/start this checkout's environment, sync it and attach; with PROMPT, start an agent on it.\n\n" +
			"PROMPT is everything after the flags, so quoting is optional. The project is the one for\n" +
			"this checkout; name another with --project (`repose attach PROJECT` attaches without syncing).",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Prompt = strings.TrimSpace(strings.Join(args, " "))
			opts.ProjectArg = g.project
			e, err := env()
			if err != nil {
				return err
			}
			return runRun(cmd.Context(), e, opts, false)
		},
	}
	cmd.Flags().StringVar(&opts.Agent, "agent", "", "claude|opencode|codex|gemini|pi")
	cmd.Flags().StringVar(&opts.Size, "size", "", "small|large|xl")
	cmd.Flags().StringVar(&opts.Name, "name", "", "project name, for a directory with no git remote")
	cmd.Flags().BoolVar(&opts.StashRemote, "stash-remote", false, "stash the guest's uncommitted changes before syncing")
	cmd.Flags().BoolVar(&opts.DiscardRemote, "discard-remote", false, "discard the guest's uncommitted changes before syncing")
	cmd.Flags().BoolVar(&opts.NoSync, "no-sync", false, "skip the git and credential sync")
	cmd.Flags().BoolVar(&opts.NoAttach, "no-attach", false, "do not attach after starting/sending the prompt")
	_ = cmd.RegisterFlagCompletionFunc("agent", cobra.FixedCompletions([]string{"claude", "opencode", "codex", "gemini", "pi"}, cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("size", cobra.FixedCompletions([]string{"small", "large", "xl"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func newAttachCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:               "attach [PROJECT]",
		Short:             "Attach to a project's tmux session (this checkout's, or PROJECT)",
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
			return runRun(cmd.Context(), e, RunOptions{ProjectArg: project}, true)
		},
	}
}

func newStartCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:               "start [PROJECT]",
		Short:             "Start a project's environment without syncing (restarts one in error)",
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
			return StartCmd(cmd.Context(), e, project)
		},
	}
}

func newStopCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var noSnapshot bool
	cmd := &cobra.Command{
		Use:               "stop [PROJECT]",
		Short:             "Snapshot and stop a project's environment",
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
			return StopCmd(cmd.Context(), e, project, !noSnapshot)
		},
	}
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "stop without taking a snapshot")
	return cmd
}

func newStatusCmd(envJSON func(*cobra.Command) (*Env, error), env func() (*Env, error), g *globalFlags) *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:               "status [PROJECT]",
		Short:             "Show a project's status",
		Args:              projectArgs,
		ValidArgsFunction: completeProject(env),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFrom(args, g)
			if err != nil {
				return err
			}
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			if !watch {
				return StatusCmd(cmd.Context(), e, project)
			}
			for {
				if err := StatusCmd(cmd.Context(), e, project); err != nil {
					return err
				}
				if err := sleepOrDone(cmd.Context(), 5*time.Second); err != nil {
					return nil
				}
			}
		},
	}
	cmd.Flags().Bool("json", false, "print the Project object as JSON")
	cmd.Flags().BoolVar(&watch, "watch", false, "refresh every 5 seconds")
	return cmd
}

func newProjectsCmd(envJSON func(*cobra.Command) (*Env, error)) *cobra.Command {
	var destroyed bool
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List every project",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			if destroyed {
				return DestroyedCmd(cmd.Context(), e)
			}
			return ProjectsCmd(cmd.Context(), e)
		},
	}
	cmd.Flags().Bool("json", false, "print as JSON")
	cmd.Flags().BoolVar(&destroyed, "destroyed", false, "list destroyed projects that can still be restored, and until when")
	return cmd
}

func newOpenCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var desktop, noBrowser bool
	var localPort int
	cmd := &cobra.Command{
		Use:   "open [PORT]",
		Short: "Forward a guest port, or the desktop, to the laptop",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			if desktop {
				return OpenDesktopCmd(cmd.Context(), e, g.project, noBrowser)
			}
			if len(args) != 1 {
				return cobraUsageError{fmt.Errorf("repose open PORT (or --desktop)")}
			}
			port, err := strconv.Atoi(args[0])
			if err != nil || port < 1 || port > 65535 {
				return cobraUsageError{fmt.Errorf("PORT must be a port number (1-65535), got %q; the project is --project NAME", args[0])}
			}
			return OpenPortCmd(cmd.Context(), e, g.project, port, localPort, noBrowser)
		},
	}
	cmd.Flags().BoolVar(&desktop, "desktop", false, "open the on-demand desktop instead of a port")
	cmd.Flags().IntVar(&localPort, "local-port", 0, "local port to bind (defaults to PORT)")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL instead of opening a browser")
	return cmd
}

func newSecretsCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	root := &cobra.Command{Use: "secrets", Short: "Manage project secrets"}
	var fromFile string
	var fromEnv bool
	set := &cobra.Command{
		Use:   "set NAME",
		Short: "Set a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			value, err := readSecretValue(args[0], fromFile, fromEnv)
			if err != nil {
				return err
			}
			return SecretsSetCmd(cmd.Context(), e, g.project, args[0], value)
		},
	}
	set.Flags().StringVar(&fromFile, "from-file", "", "read the value from a file")
	set.Flags().BoolVar(&fromEnv, "from-env", false, "read the value from $NAME")

	list := &cobra.Command{
		Use:   "list",
		Short: "List secret names",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return SecretsListCmd(cmd.Context(), e, g.project)
		},
	}
	rm := &cobra.Command{
		Use:   "rm NAME",
		Short: "Remove a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return SecretsRmCmd(cmd.Context(), e, g.project, args[0])
		},
	}
	root.AddCommand(set, list, rm)
	return root
}

func readSecretValue(name, fromFile string, fromEnv bool) ([]byte, error) {
	switch {
	case fromFile != "":
		b, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, exitf(ExitUsage, "Could not read %s: %v", fromFile, err)
		}
		return b, nil
	case fromEnv:
		v, ok := os.LookupEnv(name)
		if !ok {
			return nil, cobraUsageError{fmt.Errorf("$%s is not set", name)}
		}
		return []byte(v), nil
	default:
		if !isTerminal(os.Stdin) {
			return nil, exitf(ExitUsage, "No terminal to type %s's value into; use --from-file PATH or --from-env.", name)
		}
		_, _ = fmt.Fprintf(os.Stderr, "Value for %s (not shown): ", name)
		v, err := readHiddenLine()
		if err != nil {
			return nil, err
		}
		return v, nil
	}
}

func newConfigCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	root := &cobra.Command{Use: "config", Short: "Manage the guest's Nix configuration"}
	var showRevisions bool
	show := &cobra.Command{
		Use:   "show",
		Short: "Print the current fragment",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return ConfigShowCmd(cmd.Context(), e, g.project, showRevisions)
		},
	}
	show.Flags().BoolVar(&showRevisions, "revisions", false, "list revisions instead")

	edit := &cobra.Command{
		Use:   "edit",
		Short: "Edit the fragment in $EDITOR",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return ConfigEditCmd(cmd.Context(), e, g.project, openInEditor)
		},
	}
	apply := &cobra.Command{
		Use:   "apply [PATH]",
		Short: "Apply a fragment file (default ./repose.nix)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			path := ""
			if len(args) == 1 {
				path = args[0]
			}
			return ConfigApplyCmd(cmd.Context(), e, g.project, path)
		},
	}
	root.AddCommand(show, edit, apply)
	return root
}

func openInEditor(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// $EDITOR is often "code --wait" or "emacsclient -t": a command line,
	// not a path.
	fields := strings.Fields(editor)
	cmd := exec.Command(fields[0], append(fields[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func newSnapshotsCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	root := &cobra.Command{Use: "snapshots", Short: "Manage snapshots"}
	var asNew string
	var yes bool
	list := &cobra.Command{
		Use:   "list",
		Short: "List snapshots",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return SnapshotsListCmd(cmd.Context(), e, g.project)
		},
	}
	list.Flags().BoolVar(&g.json, "json", false, "print as JSON")
	create := &cobra.Command{
		Use:   "create",
		Short: "Take a manual snapshot",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return SnapshotsCreateCmd(cmd.Context(), e, g.project)
		},
	}
	restore := &cobra.Command{
		Use:   "restore SNAPSHOT_ID",
		Short: "Restore a snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			var confirm func() (bool, error)
			if !yes {
				confirm = func() (bool, error) {
					return askYesNo("Restore over the current volume? Anything since the snapshot is lost. [y/N] ", false, "restoring in place")
				}
			}
			return SnapshotsRestoreCmd(cmd.Context(), e, g.project, args[0], asNew, confirm)
		},
	}
	restore.Flags().StringVar(&asNew, "as-new", "", "restore into a new project instead of replacing this one")
	restore.Flags().BoolVar(&yes, "yes", false, "skip the confirmation")
	root.AddCommand(list, create, restore)
	return root
}

func newDestroyCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var yes, wait bool
	cmd := &cobra.Command{
		Use:               "destroy [PROJECT]",
		Short:             "Destroy a project (a final snapshot is kept for 30 days)",
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
			var confirm func(string) (bool, error)
			if !yes {
				confirm = func(prompt string) (bool, error) { return askYesNo(prompt, false, "destroying") }
			}
			return DestroyCmd(cmd.Context(), e, project, yes, wait, confirm)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait until the destroy is done and report how it ended (for scripts)")
	return cmd
}

func newRestoreCmd(env func() (*Env, error)) *cobra.Command {
	var as, snapshot string
	cmd := &cobra.Command{
		Use:   "restore [NAME]",
		Short: "Bring back a destroyed project from its newest snapshot (kept 30 days)",
		Long: "Restores NAME, a project you destroyed in the last 30 days (or one that still exists), from its\n" +
			"newest snapshot into a new project called NAME, or --as NEW-NAME when that name is in use.\n" +
			"Without NAME, inside a checkout, it restores the destroyed project with this checkout's remote.\n" +
			"`repose projects --destroyed` lists what can be restored. `repose snapshots restore` still\n" +
			"restores a given snapshot over a stopped project in place.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return cobraUsageError{fmt.Errorf("%s takes one NAME, got %d arguments: %s", cmd.CommandPath(), len(args), strings.Join(args, " "))}
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return destroyedSlugsForCompletion(env), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			var ask func(string) (string, error)
			if isTerminal(os.Stdin) {
				ask = func(prompt string) (string, error) {
					_, _ = fmt.Fprint(os.Stderr, prompt)
					line, err := readLine()
					if err != nil && line == "" {
						_, _ = fmt.Fprintln(os.Stderr)
						return "", nil
					}
					return line, nil
				}
			}
			return RestoreCmd(cmd.Context(), e, name, as, snapshot, ask)
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "name for the restored project (default: its old name)")
	cmd.Flags().StringVar(&snapshot, "snapshot", "", "restore this snapshot instead of the newest (`repose snapshots list --project ID` lists them)")
	return cmd
}

func newResizeCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "resize SIZE",
		Short:  "Grow the project's volume (e.g. 80G)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bytes, err := parseSize(args[0])
			if err != nil {
				return cobraUsageError{err}
			}
			e, err := env()
			if err != nil {
				return err
			}
			return ResizeCmd(cmd.Context(), e, g.project, bytes)
		},
	}
	return cmd
}

func newLogsCmd(envJSON func(*cobra.Command) (*Env, error), env func() (*Env, error), g *globalFlags) *cobra.Command {
	var kind, since string
	var follow bool
	cmd := &cobra.Command{
		Use:               "logs [PROJECT]",
		Short:             "Show a project's logs",
		Args:              projectArgs,
		ValidArgsFunction: completeProject(env),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFrom(args, g)
			if err != nil {
				return err
			}
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			return LogsCmd(cmd.Context(), e, project, kind, since, follow, nil)
		},
	}
	cmd.Flags().Bool("json", false, "print each line as JSON")
	cmd.Flags().StringVar(&kind, "kind", "", "console|build|ops")
	cmd.Flags().StringVar(&since, "since", "", "e.g. 1h")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "poll for new lines every 2s")
	_ = cmd.RegisterFlagCompletionFunc("kind", cobra.FixedCompletions([]string{"console", "build", "ops"}, cobra.ShellCompDirectiveNoFileComp))
	return cmd
}

func newEventsCmd(envJSON func(*cobra.Command) (*Env, error), env func() (*Env, error), g *globalFlags) *cobra.Command {
	var since string
	var follow bool
	cmd := &cobra.Command{
		Use:               "events [PROJECT]",
		Short:             "Show a project's events",
		Args:              projectArgs,
		ValidArgsFunction: completeProject(env),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := projectFrom(args, g)
			if err != nil {
				return err
			}
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			if since == "" {
				since = "24h"
			}
			return EventsCmd(cmd.Context(), e, project, since, follow, nil)
		},
	}
	cmd.Flags().Bool("json", false, "print each event as JSON")
	cmd.Flags().StringVar(&since, "since", "24h", "e.g. 24h")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "poll for new events every 10s")
	return cmd
}

func newNotifyCmd(env func() (*Env, error)) *cobra.Command {
	root := &cobra.Command{Use: "notify", Short: "Notification settings"}
	var emailFlag, ntfyFlag string
	set := &cobra.Command{
		Use:   "set",
		Short: "Change notification settings",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if emailFlag != "" && emailFlag != "on" && emailFlag != "off" {
				return cobraUsageError{fmt.Errorf("--email is on or off, got %q", emailFlag)}
			}
			e, err := env()
			if err != nil {
				return err
			}
			var email *bool
			if emailFlag != "" {
				b := emailFlag == "on"
				email = &b
			}
			var ntfy *string
			if ntfyFlag != "" {
				ntfy = &ntfyFlag
			}
			return NotifySetCmd(cmd.Context(), e, email, ntfy)
		},
	}
	set.Flags().StringVar(&emailFlag, "email", "", "on|off")
	set.Flags().StringVar(&ntfyFlag, "ntfy", "", "a URL, or none")
	test := &cobra.Command{
		Use:   "test",
		Short: "Send a test notification on every configured channel",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return NotifyTestCmd(cmd.Context(), e)
		},
	}
	root.AddCommand(set, test)
	return root
}

// newVersionCmd prints "repose <version> (herakraft)" (DECISIONS I-15: the
// suffix lets a user tell this binary apart from Arch's unrelated
// `repose` package on the same PATH).
func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("repose %s (herakraft)\n", version)
			return nil
		},
	}
}

func newMCPCmd() *cobra.Command {
	root := &cobra.Command{Use: "mcp", Short: "MCP helpers (reserved)"}
	root.AddCommand(&cobra.Command{
		Use:   "forward",
		Short: "Forward a laptop-bound MCP server into the guest (not available yet)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(NotAvailableMessage("repose mcp forward"))
			return nil
		},
	})
	return root
}

func newBrowserCmd() *cobra.Command {
	root := &cobra.Command{Use: "browser", Short: "Browser helpers (reserved)"}
	root.AddCommand(&cobra.Command{
		Use:   "bridge",
		Short: "Bridge the laptop's Chrome into the guest (not available yet)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(NotAvailableMessage("repose browser bridge"))
			return nil
		},
	})
	return root
}

// Interactive helpers. Kept small and separate from the pure command
// functions above so those stay unit-testable without a terminal.

func goos() string {
	if v := os.Getenv("REPOSE_TEST_GOOS"); v != "" {
		return v
	}
	return runtime.GOOS
}

// askYesNo asks prompt on stderr and reads the answer from stdin; an
// empty answer is defaultYes, as the [Y/n] or [y/N] in the prompt says.
// v0.1.4's helper treated an empty answer as yes even for [y/N]. Without
// a terminal on stdin there is nobody to ask, which is a usage error
// naming --yes rather than a silent default.
func askYesNo(prompt string, defaultYes bool, what string) (bool, error) {
	if !isTerminal(os.Stdin) {
		return false, exitf(ExitUsage, "No terminal to confirm %s on; pass --yes.", what)
	}
	_, _ = fmt.Fprint(os.Stderr, prompt)
	line, err := readLine()
	if err != nil && line == "" {
		_, _ = fmt.Fprintln(os.Stderr)
		return false, nil
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "":
		return defaultYes, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func readLine() (string, error) {
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

// readHiddenLine reads a line with the terminal's echo off (stty, which
// every macOS and Linux laptop has), so a secret typed at `repose
// secrets set` never shows on screen or in a screen recording. Echo comes
// back on however the read ends, Ctrl-C included.
func readHiddenLine() ([]byte, error) {
	if runtime.GOOS != "windows" && stty("-echo") == nil {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)
		done := make(chan struct{})
		go func() {
			select {
			case <-sig:
				_ = stty("echo")
				_, _ = fmt.Fprintln(os.Stderr)
				os.Exit(130)
			case <-done:
			}
		}()
		defer func() {
			close(done)
			signal.Stop(sig)
			_ = stty("echo")
			_, _ = fmt.Fprintln(os.Stderr)
		}()
	}
	line, err := readLine()
	if err != nil && line == "" {
		return nil, exitf(ExitUsage, "No value given.")
	}
	return []byte(line), nil
}

func stty(arg string) error {
	cmd := exec.Command("stty", arg)
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.TrimSuffix(s, "B")
	s = strings.TrimSuffix(s, "I")
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "T"):
		mult = 1 << 40
		s = strings.TrimSuffix(s, "T")
	case strings.HasSuffix(s, "G"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "M")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%q is not a size like 80G", s)
	}
	return n * mult, nil
}
