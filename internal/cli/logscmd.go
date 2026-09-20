package cli

import (
	"context"
	"fmt"
	"time"
)

// LogsCmd implements `repose logs [--kind console|build|ops] [--since 1h]
// [--follow]`. follow polls every 2s (07-cli.md §5.10); poll is injected
// so tests do not sleep in a loop.
func LogsCmd(ctx context.Context, e *Env, projectArg, kind, since string, follow bool, poll func()) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	cur := since
	for {
		lines, err := e.Client.ProjectLogs(ctx, project.ID, kind, cur)
		if err != nil {
			return err
		}
		for _, l := range lines {
			if e.JSON {
				if err := writeJSONOut(e.Out, l); err != nil {
					return err
				}
				continue
			}
			_, _ = fmt.Fprintf(e.Out, "%s %s %s\n", l.TS.Format(time.RFC3339), l.Kind, l.Line)
		}
		if !follow {
			return nil
		}
		if len(lines) > 0 {
			cur = lines[len(lines)-1].TS.Format(time.RFC3339)
		}
		if poll != nil {
			poll()
		} else if err := sleepOrDone(ctx, 2*time.Second); err != nil {
			return err
		}
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// EventsCmd implements `repose events [--since 24h] [--follow]`
// (07-cli.md's I-8 addition).
func EventsCmd(ctx context.Context, e *Env, projectArg, since string, follow bool, poll func()) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	cur := since
	for {
		events, err := e.Client.ListEvents(ctx, project.ID, cur)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if e.JSON {
				if err := writeJSONOut(e.Out, ev); err != nil {
					return err
				}
				continue
			}
			_, _ = fmt.Fprintf(e.Out, "%s\t%s\t%s\t%s\n", ev.TS.Format(time.RFC3339), ev.Agent, ev.Kind, ev.Summary)
		}
		if !follow {
			return nil
		}
		if len(events) > 0 {
			cur = events[len(events)-1].TS.Format(time.RFC3339)
		}
		if poll != nil {
			poll()
		} else if err := sleepOrDone(ctx, 10*time.Second); err != nil {
			return err
		}
	}
}
