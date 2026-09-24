package admin

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/abuse"
)

// abuseCmd is `repose-admin abuse list [--all]` and `abuse clear PROJECT`
// (DECISIONS I-239). The api stops a guest whose process samples name a
// miner and records the stop; list shows those stops, clear lifts a
// project's hold and its strikes. Suspending the user stays
// `users suspend`, a separate decision.
func (e *Env) abuseCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return ErrUsage
	}
	if err := e.connect(ctx); err != nil {
		return err
	}
	switch args[0] {
	case "list":
		all := len(args) > 1 && args[1] == "--all"
		q := `select a.ts, p.slug, u.handle, a.kind, coalesce(a.detail->>'process', ''), a.hold, a.cleared_at, coalesce(a.cleared_by, ''), p.id
			from abuse_events a join projects p on p.id = a.project_id join users u on u.id = a.user_id`
		if !all {
			q += " where a.cleared_at is null"
		}
		q += " order by a.ts desc limit 200"
		rows, err := e.pool.Query(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		out := [][]string{{"WHEN", "PROJECT", "OWNER", "KIND", "PROCESS", "HOLD", "CLEARED", "BY", "PROJECT ID"}}
		for rows.Next() {
			var (
				ts            time.Time
				slug, handle  string
				kind, process string
				hold          bool
				cleared       *time.Time
				by            string
				pid           uuid.UUID
			)
			if err := rows.Scan(&ts, &slug, &handle, &kind, &process, &hold, &cleared, &by, &pid); err != nil {
				return err
			}
			out = append(out, []string{fmtTime(&ts), slug, handle, kind, process, strconv.FormatBool(hold), fmtTime(cleared), orDash(by), pid.String()})
		}
		if err := rows.Err(); err != nil {
			return err
		}
		e.table(out)
		return nil
	case "clear":
		if len(args) < 2 {
			return fmt.Errorf("%w: abuse clear PROJECT", ErrUsage)
		}
		p, err := e.findProject(ctx, args[1])
		if err != nil {
			return err
		}
		n, err := abuse.Clear(ctx, e.pool, p.ID, e.Actor)
		if err != nil {
			return err
		}
		if _, err := e.audited(ctx, "abuse_clear", p.ID.String(), map[string]any{"cleared": n}); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(e.Stdout, "%s: %d abuse stop(s) cleared; it can be started again\n", p.Slug, n)
		return nil
	}
	return fmt.Errorf("%w: abuse list [--all] | abuse clear PROJECT", ErrUsage)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
