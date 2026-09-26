package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// `repose ps` (DECISIONS I-274): the project's tmux windows, the way
// `docker ps` lists containers. One ssh over the project's multiplexed
// connection runs `tmux list-windows`; the command column is tmux's
// pane_current_command, the process name only, never its arguments. The
// output goes to the user's own terminal and nowhere else.

// PsWindow is one tmux window, also `repose ps --json`'s element.
type PsWindow struct {
	Index    int       `json:"index"`
	Name     string    `json:"name"`
	Command  string    `json:"command"`
	Current  bool      `json:"current"`
	Activity time.Time `json:"activity"`
	IdleSecs int64     `json:"idle_seconds"`
}

// psScript prints the guest's clock, then one tab-separated line per
// window of session slug. The clock comes from the same machine as the
// activity times, so a laptop clock that is off does not skew "2m ago".
func psScript(slug string) string {
	return fmt.Sprintf("date +%%s && tmux list-windows -t %s -F '#{window_index}\t#{window_name}\t#{pane_current_command}\t#{window_activity}\t#{window_active}'",
		shQuote("="+slug))
}

// parsePs reads psScript's output.
func parsePs(out string) ([]PsWindow, error) {
	lines := nonEmptyLines(out)
	if len(lines) == 0 {
		return nil, fmt.Errorf("no output")
	}
	now, err := strconv.ParseInt(strings.TrimSpace(lines[0]), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unexpected clock %q", lines[0])
	}
	var ws []PsWindow
	for _, l := range lines[1:] {
		f := strings.Split(l, "\t")
		if len(f) != 5 {
			return nil, fmt.Errorf("unexpected line with %d fields", len(f))
		}
		idx, _ := strconv.Atoi(f[0])
		act, _ := strconv.ParseInt(f[3], 10, 64)
		idle := now - act
		if idle < 0 {
			idle = 0
		}
		ws = append(ws, PsWindow{
			Index: idx, Name: f[1], Command: f[2], Current: f[4] == "1",
			Activity: time.Unix(act, 0).UTC(), IdleSecs: idle,
		})
	}
	return ws, nil
}

// idleAgo is the ACTIVE column: "now" under a minute, then minutes,
// hours and days, as docker ps rounds.
func idleAgo(secs int64) string {
	switch d := time.Duration(secs) * time.Second; {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
}

func newPsCmd(envJSON func(*cobra.Command) (*Env, error), env func() (*Env, error), g *globalFlags) *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "ps [PROJECT]",
		Short: "List the project's tmux windows: what runs in each and when it last printed",
		Long: "Lists the tmux windows of PROJECT (this checkout's, by default): each window's number and\n" +
			"name, the program in its active pane (its name only), and when the window last printed\n" +
			"something. The current window, the one `repose attach` opens on, is marked with *.",
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
			if quiet && e.JSON {
				return cobraUsageError{fmt.Errorf("-q and --json are two different outputs; pass one")}
			}
			e.Quiet = quiet
			return PsCmd(cmd.Context(), e, project)
		},
	}
	cmd.Flags().Bool("json", false, "print the windows as JSON")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "print only the window names, one per line")
	return cmd
}

// Exit status of psScript's tmux when the session does not exist.
const psExitNoSession = 1

// PsCmd implements `repose ps [PROJECT]`.
func PsCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireRunningProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	target, err := connect(ctx, e, project)
	if err != nil {
		return err
	}
	out, err := runSSH(ctx, target, psScript(project.Slug), nil)
	var se *sshError
	if errors.As(err, &se) && se.ExitCode == psExitNoSession && strings.Contains(se.Stderr, "can't find session") {
		return exitf(ExitGeneric, "%s has no tmux session. `repose attach %s` starts one.", project.Slug, project.Slug)
	}
	if err != nil {
		return stepFailed("list the tmux windows on "+project.Slug, err, "")
	}
	ws, err := parsePs(string(out))
	if err != nil {
		return stepFailed("read the tmux windows on "+project.Slug, err, "")
	}
	switch {
	case e.JSON:
		if ws == nil {
			ws = []PsWindow{}
		}
		return writeJSONOut(e.Out, ws)
	case e.Quiet:
		for _, w := range ws {
			_, _ = fmt.Fprintln(e.Out, w.Name)
		}
		return nil
	}
	tw := tabwriter.NewWriter(e.Out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "WINDOW\tCOMMAND\tACTIVE")
	for _, w := range ws {
		mark := " "
		if w.Current {
			mark = "*"
		}
		_, _ = fmt.Fprintf(tw, "%d:%s%s\t%s\t%s\n", w.Index, w.Name, mark, w.Command, idleAgo(w.IdleSecs))
	}
	return tw.Flush()
}
