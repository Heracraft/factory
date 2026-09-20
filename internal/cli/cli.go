package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
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
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	if usageErr, ok := err.(cobraUsageError); ok {
		_, _ = fmt.Fprintln(os.Stderr, usageErr.Error())
		return ExitUsage
	}
	return exitCodeFor(err, os.Stderr)
}

// cobraUsageError marks an error as a plain usage mistake (bad flags,
// wrong arg count) rather than a command failure.
type cobraUsageError struct{ error }

type globalFlags struct {
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
	}
	root.PersistentFlags().StringVar(&g.project, "project", "", "project id or slug (or $REPOSE_PROJECT)")
	root.PersistentFlags().StringVar(&g.apiURL, "api-url", "", "api base url (or $REPOSE_API_URL)")
	root.PersistentFlags().BoolVarP(&g.verbose, "verbose", "v", false, "debug logging to stderr")

	env := func() (*Env, error) {
		e, err := newEnv(g.apiURL, g.json, g.verbose)
		if err != nil {
			return nil, err
		}
		e.Client.HTTP = e.httpClient
		return e, nil
	}
	envJSON := func(cmd *cobra.Command) (*Env, error) {
		json, _ := cmd.Flags().GetBool("json")
		g.json = json
		return env()
	}

	root.AddCommand(
		newLoginCmd(),
		newLogoutCmd(env),
		newRunCmd(env, g),
		newAttachCmd(env, g),
		newStartCmd(env, g),
		newStopCmd(env, g),
		newStatusCmd(envJSON, g),
		newOpenCmd(env, g),
		newSecretsCmd(env, g),
		newConfigCmd(env, g),
		newSnapshotsCmd(env, g),
		newDestroyCmd(env, g),
		newLogsCmd(envJSON, g),
		newProjectsCmd(envJSON),
		newEventsCmd(envJSON, g),
		newNotifyCmd(env),
		newResizeCmd(env, g),
		newVersionCmd(version),
		newCompletionCmd(),
		newMCPCmd(),
		newBrowserCmd(),
	)
	return root
}

func newLoginCmd() *cobra.Command {
	var noBrowser, browser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with Logto",
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
		Short: "Create/start the project's guest and attach, optionally starting an agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.Prompt = args[0]
			}
			opts.ProjectArg = g.project
			e, err := env()
			if err != nil {
				return err
			}
			opts.AskPush = interactiveAskPush(e.Cwd)
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
	return cmd
}

func newAttachCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "attach",
		Short: "Attach to the project's tmux session",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return runRun(cmd.Context(), e, RunOptions{ProjectArg: g.project}, true)
		},
	}
}

func newStartCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the guest without syncing",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return StartCmd(cmd.Context(), e, g.project)
		},
	}
}

func newStopCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var noSnapshot bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Snapshot and stop the guest",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return StopCmd(cmd.Context(), e, g.project, !noSnapshot)
		},
	}
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "stop without taking a snapshot")
	return cmd
}

func newStatusCmd(envJSON func(*cobra.Command) (*Env, error), g *globalFlags) *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the project's status",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			if !watch {
				return StatusCmd(cmd.Context(), e, g.project)
			}
			for {
				if err := StatusCmd(cmd.Context(), e, g.project); err != nil {
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
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "List every project",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			return ProjectsCmd(cmd.Context(), e)
		},
	}
	cmd.Flags().Bool("json", false, "print as JSON")
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
			if err != nil {
				return cobraUsageError{fmt.Errorf("PORT must be a number: %v", err)}
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
		return os.ReadFile(fromFile)
	case fromEnv:
		v, ok := os.LookupEnv(name)
		if !ok {
			return nil, cobraUsageError{fmt.Errorf("$%s is not set", name)}
		}
		return []byte(v), nil
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Value for %s: ", name)
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
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func newSnapshotsCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	root := &cobra.Command{Use: "snapshots", Short: "Manage snapshots"}
	var asNew string
	list := &cobra.Command{
		Use:   "list",
		Short: "List snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return SnapshotsListCmd(cmd.Context(), e, g.project)
		},
	}
	create := &cobra.Command{
		Use:   "create",
		Short: "Take a manual snapshot",
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
			return SnapshotsRestoreCmd(cmd.Context(), e, g.project, args[0], asNew, interactiveConfirm("Restore over the current volume? [y/N] "))
		},
	}
	restore.Flags().StringVar(&asNew, "as-new", "", "restore into a new project instead of replacing this one")
	root.AddCommand(list, create, restore)
	return root
}

func newDestroyCmd(env func() (*Env, error), g *globalFlags) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "destroy",
		Short: "Destroy the project",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := env()
			if err != nil {
				return err
			}
			return DestroyCmd(cmd.Context(), e, g.project, yes, interactiveTypedConfirm)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the typed confirmation")
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

func newLogsCmd(envJSON func(*cobra.Command) (*Env, error), g *globalFlags) *cobra.Command {
	var kind, since string
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show project logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			return LogsCmd(cmd.Context(), e, g.project, kind, since, follow, nil)
		},
	}
	cmd.Flags().Bool("json", false, "print each line as JSON")
	cmd.Flags().StringVar(&kind, "kind", "", "console|build|ops")
	cmd.Flags().StringVar(&since, "since", "", "e.g. 1h")
	cmd.Flags().BoolVar(&follow, "follow", false, "poll for new lines every 2s")
	return cmd
}

func newEventsCmd(envJSON func(*cobra.Command) (*Env, error), g *globalFlags) *cobra.Command {
	var since string
	var follow bool
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Show project events",
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := envJSON(cmd)
			if err != nil {
				return err
			}
			if since == "" {
				since = "24h"
			}
			return EventsCmd(cmd.Context(), e, g.project, since, follow, nil)
		},
	}
	cmd.Flags().Bool("json", false, "print each event as JSON")
	cmd.Flags().StringVar(&since, "since", "24h", "e.g. 24h")
	cmd.Flags().BoolVar(&follow, "follow", false, "poll for new events every 10s")
	return cmd
}

func newNotifyCmd(env func() (*Env, error)) *cobra.Command {
	root := &cobra.Command{Use: "notify", Short: "Notification settings"}
	var emailFlag, ntfyFlag string
	set := &cobra.Command{
		Use:   "set",
		Short: "Change notification settings",
		RunE: func(cmd *cobra.Command, args []string) error {
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

// interactiveAskPush is 07-cli.md §5.5c's "Commit H is not on origin. Push
// <branch> now? [Y/n]": on yes, it actually runs the push (from repoDir,
// which git resolves to the repo root on its own) and reports whether it
// did.
func interactiveAskPush(repoDir string) func(commit, branch string) (bool, error) {
	return func(commit, branch string) (bool, error) {
		_, _ = fmt.Fprintf(os.Stderr, "Commit %s is not on origin. Push %s now? [Y/n] ", commit, branch)
		yes, err := interactiveConfirm("")()
		if err != nil || !yes {
			return false, err
		}
		if _, err := gitCmd(repoDir, "push", "origin", branch); err != nil {
			return false, fmt.Errorf("git push origin %s: %w", branch, err)
		}
		return true, nil
	}
}

func interactiveConfirm(prompt string) func() (bool, error) {
	return func() (bool, error) {
		if prompt != "" {
			_, _ = fmt.Fprint(os.Stderr, prompt)
		}
		line, _ := readLine()
		line = strings.TrimSpace(strings.ToLower(line))
		return line == "" || line == "y" || line == "yes", nil
	}
}

func interactiveTypedConfirm(slug string) (string, error) {
	_, _ = fmt.Fprintf(os.Stderr, "Type %q to destroy it: ", slug)
	return readLine()
}

func readLine() (string, error) {
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

func readHiddenLine() ([]byte, error) {
	// A real TTY would use golang.org/x/term.ReadPassword; kept to a
	// plain read here so this file has no additional build-tagged
	// terminal dependency, and secrets set is still tested via
	// --from-file/--from-env, which do not go through this path.
	line, err := readLine()
	return []byte(line), err
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "G"):
		mult = 1 << 30
		s = strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult = 1 << 20
		s = strings.TrimSuffix(s, "M")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size like 80G", s)
	}
	return n * mult, nil
}
