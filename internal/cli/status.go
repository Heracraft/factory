package cli

import (
	"context"
	"fmt"
	"io"
	"time"
)

// StatusCmd implements `repose status [--json]` (07-cli.md §5.7).
func StatusCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if e.JSON {
		return writeJSONOut(e.Out, project)
	}
	route, _ := e.Client.ProjectRoute(ctx, project.ID)
	snaps, _ := e.Client.ListSnapshots(ctx, project.ID)
	events, _ := e.Client.ListEvents(ctx, project.ID, "")
	writeStatusLines(e.Out, project, route, snaps, events)
	return nil
}

// ProjectsCmd implements `repose projects`: one status first-line per
// project, ignoring cwd.
func ProjectsCmd(ctx context.Context, e *Env) error {
	projects, err := e.Client.ListProjects(ctx)
	if err != nil {
		return err
	}
	if e.JSON {
		return writeJSONOut(e.Out, projects)
	}
	for _, p := range projects {
		fmt.Fprintln(e.Out, statusFirstLine(&p))
	}
	return nil
}

func writeStatusLines(w io.Writer, p *Project, route *Route, snaps []Snapshot, events []Event) {
	fmt.Fprintln(w, statusFirstLine(p))
	if route != nil {
		disk := ""
		if p.VolumeBytes > 0 {
			disk = fmt.Sprintf("%s/%s", humanBytes(p.DiskUsedBytes), humanBytes(p.VolumeBytes))
		}
		fmt.Fprintf(w, "  host %s   ip %s   disk %s   snapshot %s\n", route.HostID, route.GuestIP, disk, snapshotAge(snaps))
	}
	if p.Signals != nil {
		fmt.Fprintf(w, "  sessions %d   tmux clients %d   docker %d\n", p.Signals.SSHSessions, p.Signals.TmuxClients, p.Signals.Docker)
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		agent := last.Agent
		if agent != "" {
			agent += " "
		}
		fmt.Fprintf(w, "  last event %s: %s%s %q\n", humanAge(last.TS), agent, last.Kind, last.Summary)
	}
}

func statusFirstLine(p *Project) string {
	uptime := ""
	if p.StartedAt != nil {
		uptime = humanDuration(time.Since(*p.StartedAt))
	}
	agentState := ""
	if p.Signals != nil {
		for _, a := range p.Signals.Agents {
			agentState = fmt.Sprintf("%s: %s", a.Agent, a.State)
			break
		}
	}
	return fmt.Sprintf("%-10s %-6s %-9s %-7s %-20s today $%.2f   month $%.2f",
		p.Slug, p.Class, p.State, uptime, agentState, centsToDollars(p.CostTodayCents), centsToDollars(p.CostMonthCents))
}

func centsToDollars(c int64) float64 { return float64(c) / 100 }

func humanDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func humanAge(t time.Time) string {
	d := time.Since(t).Round(time.Minute)
	if d < time.Minute {
		return "just now"
	}
	return humanDuration(d) + " ago"
}

func snapshotAge(snaps []Snapshot) string {
	if len(snaps) == 0 {
		return "none"
	}
	return humanAge(snaps[len(snaps)-1].CreatedAt)
}
