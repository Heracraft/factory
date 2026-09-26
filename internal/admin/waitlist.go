package admin

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"time"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/api/waitlist"
	"github.com/heracraft/repose/internal/db"
)

// waitlistCmd is `repose-admin waitlist list` and `waitlist admit
// HANDLE | --next N` (DECISIONS I-269). The api admits the queue by itself
// as room appears; admit is for letting someone in ahead of that (a
// tester, a partner) or the next N by hand after a host is added. Either
// way the user gets the same one email, sent by the api's outbox.
func (e *Env) waitlistCmd(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return ErrUsage
	}
	if err := e.connect(ctx); err != nil {
		return err
	}
	switch args[0] {
	case "list":
		rows, err := store.ListWaitlist(ctx, e.pool)
		if err != nil {
			return err
		}
		out := [][]string{{"POSITION", "HANDLE", "JOINED", "ADMITTED", "BY"}}
		for _, r := range rows {
			pos := "-"
			if r.Position > 0 {
				pos = strconv.Itoa(r.Position)
			}
			by := ""
			if r.AdmittedBy != nil {
				by = *r.AdmittedBy
			}
			out = append(out, []string{pos, r.Handle, fmtTime(&r.JoinedAt), fmtTime(r.AdmittedAt), orDash(by)})
		}
		e.table(out)
		return nil
	case "admit":
		var nextFlag *int
		fs, err := flagsFor("waitlist admit", args[1:], func(fs *flag.FlagSet) {
			nextFlag = fs.Int("next", 0, "admit the next N waiting users")
		})
		if err != nil {
			return err
		}
		next := *nextFlag
		if (next > 0) == (fs.NArg() == 1) || fs.NArg() > 1 || next < 0 {
			return fmt.Errorf("%w: waitlist admit HANDLE | waitlist admit --next N", ErrUsage)
		}
		var handles []string
		if next > 0 {
			waiting, err := store.ListWaiting(ctx, e.pool)
			if err != nil {
				return err
			}
			for i := 0; i < next && i < len(waiting); i++ {
				handles = append(handles, waiting[i].Handle)
			}
			if len(handles) == 0 {
				_, _ = fmt.Fprintln(e.Stdout, "nobody is waiting") // stdout
				return nil
			}
		} else {
			handles = []string{fs.Arg(0)}
		}
		for _, h := range handles {
			u, err := e.findUser(ctx, h)
			if err != nil {
				return err
			}
			ok, err := waitlist.Admit(ctx, e.pool, u.ID, e.Actor, time.Now())
			if err != nil {
				return err
			}
			if !ok {
				cur, err := store.GetWaitlistEntry(ctx, e.pool, u.ID)
				switch {
				case errors.Is(err, db.ErrNotFound):
					_, _ = fmt.Fprintf(e.Stdout, "%s is not on the waitlist\n", u.Handle) // stdout
				case err != nil:
					return err
				default:
					_, _ = fmt.Fprintf(e.Stdout, "%s was already admitted at %s\n", u.Handle, fmtTime(cur.AdmittedAt)) // stdout
				}
				continue
			}
			if _, err := e.audited(ctx, "waitlist_admit", u.Handle, nil); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(e.Stdout, "%s admitted; the email goes out from the api's outbox\n", u.Handle) // stdout
		}
		return nil
	}
	return fmt.Errorf("%w: waitlist list | waitlist admit HANDLE | waitlist admit --next N", ErrUsage)
}
