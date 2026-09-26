// Package waitlist is the capacity waitlist (DECISIONS I-269). Memory is
// never oversubscribed and hosts are added by hand when reserved memory
// passes 80 percent (DESIGN.md §4, RUNBOOK HostMemory80), so a burst of
// sign-ups can fill the fleet before the next host is up. From that line
// on, a user asking for their first project is refused with `waitlisted`
// and put in a queue; the api admits the queue oldest first as room
// appears and emails each user once when they are in.
//
// Only a first project is gated. A user who has ever had a project, who
// was admitted, or whose account is exempt is never waitlisted: they may
// still meet plain `capacity` when no host fits.
package waitlist

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/scheduler"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
)

// DefaultPercent is the reserved-memory line, the HostMemory80 alert's.
const DefaultPercent = 80

// AdmitWindow is how long an admitted user who has not created a project
// yet still counts against capacity, so a burst of admissions cannot
// promise the same room twice. After it the room is assumed not taken up;
// the user stays admitted.
const AdmitWindow = 72 * time.Hour

// PendingClass is the class an admitted user is assumed to ask for: the
// api's default class for POST /projects.
const PendingClass = "large"

// Kind is the event (and email) an admission raises.
const Kind = "waitlist_admitted"

// Subject is the admission email's subject line.
const Subject = "There is room for you on repose"

// Summary is the admission email's body: fixed text, nothing the user or
// any guest wrote.
const Summary = "repose has room for your first machine now. Run `repose run` again in your project's checkout to create it."

// Fleet is the memory picture the gate and the admitter share.
type Fleet struct {
	// Usable is RAM minus the host reserve over ready, undrained hosts.
	Usable int64
	// Reserved is memory promised to guests on those hosts.
	Reserved int64
	// Pending is admitted users inside AdmitWindow with no project yet.
	Pending int
}

// Projected is what the fleet would have reserved with extra more bytes
// and every pending admission taken up.
func (f Fleet) Projected(extra int64) int64 {
	return f.Reserved + int64(f.Pending)*scheduler.ClassRAM(PendingClass) + extra
}

// Fits reports whether extra more bytes stay at or under percent of the
// usable memory. A fleet with no usable memory fits nothing.
func (f Fleet) Fits(percent int, extra int64) bool {
	return f.Usable > 0 && f.Projected(extra)*100 <= int64(percent)*f.Usable
}

// ReadFleet reads the picture at now.
func ReadFleet(ctx context.Context, q store.Querier, now time.Time) (Fleet, error) {
	var f Fleet
	err := q.QueryRow(ctx, `select
		coalesce(sum(h.mem_bytes - case when h.mem_bytes >= (128::bigint<<30) then 16::bigint<<30 else 8::bigint<<30 end), 0)::bigint,
		coalesce(sum(r.reserved_bytes), 0)::bigint
		from hosts h left join host_reservations r on r.host_id = h.id
		where h.state = 'ready' and not h.draining`).Scan(&f.Usable, &f.Reserved)
	if err != nil {
		return f, err
	}
	if f.Usable < 0 {
		f.Usable = 0
	}
	err = q.QueryRow(ctx, `select count(*) from waitlist w
		where w.admitted_at > $1 and not exists (select 1 from projects p where p.user_id = w.user_id)`, now.Add(-AdmitWindow)).Scan(&f.Pending)
	return f, err
}

// Gate is the check before a first project is created.
type Gate struct {
	Pool *db.Pool
	// Percent is WAITLIST_PERCENT; 0 turns the waitlist off.
	Percent int
	M       *metrics.M
}

// Check returns the user's waitlist entry when their create must wait,
// and nil when it may go ahead. A user who must wait and is not on the
// list yet is added; a user already waiting keeps their place, even when
// room has appeared, since the admitter lets the queue in in order.
// joined says whether this call added them.
func (g *Gate) Check(ctx context.Context, u *store.User, class string, now time.Time) (e *store.WaitlistEntry, joined bool, err error) {
	if g == nil || g.Percent <= 0 || u.BillingStatus == "exempt" {
		return nil, false, nil
	}
	var hadProject bool
	if err := g.Pool.QueryRow(ctx, "select exists(select 1 from projects where user_id = $1)", u.ID).Scan(&hadProject); err != nil {
		return nil, false, err
	}
	if hadProject {
		return nil, false, nil
	}
	cur, err := store.GetWaitlistEntry(ctx, g.Pool, u.ID)
	switch {
	case errors.Is(err, db.ErrNotFound):
	case err != nil:
		return nil, false, err
	case cur.AdmittedAt != nil:
		return nil, false, nil
	default:
		return cur, false, nil
	}
	f, err := ReadFleet(ctx, g.Pool, now)
	if err != nil {
		return nil, false, err
	}
	// No usable host at all is an outage or an empty dev fleet, not a
	// full one: placement answers that with plain `capacity`.
	if f.Usable == 0 {
		return nil, false, nil
	}
	var waiting int
	if err := g.Pool.QueryRow(ctx, `select count(*) from waitlist w join users u on u.id = w.user_id
		where w.admitted_at is null and u.suspended_at is null and u.cancelled_at is null and u.deleted_at is null`).Scan(&waiting); err != nil {
		return nil, false, err
	}
	// Nobody waiting and room below the line: go ahead. With a queue, a
	// newcomer joins its end rather than taking the room the next
	// admission is about to hand out.
	if waiting == 0 && f.Fits(g.Percent, scheduler.ClassRAM(class)) {
		return nil, false, nil
	}
	tag, err := g.Pool.Exec(ctx, "insert into waitlist (user_id, joined_at) values ($1, $2) on conflict (user_id) do nothing", u.ID, now)
	if err != nil {
		return nil, false, err
	}
	e, err = store.GetWaitlistEntry(ctx, g.Pool, u.ID)
	if err != nil {
		return nil, false, err
	}
	if e.AdmittedAt != nil {
		return nil, false, nil // admitted between the two reads
	}
	joined = tag.RowsAffected() == 1
	if joined && g.M != nil {
		g.M.WaitlistJoinedTotal.Inc()
	}
	return e, joined, nil
}

// Message is the `waitlisted` error's message, the whole sentence a CLI
// that does not know the code prints as it is.
func Message(position int, email string) string {
	if email == "" {
		return fmt.Sprintf("repose is at capacity. You're number %d on the waitlist. Run `repose run` again later.", position)
	}
	return fmt.Sprintf("repose is at capacity. You're number %d on the waitlist; we'll email %s when there's room.", position, email)
}

// Admit lets one waiting user in: admitted_at is set and the admission
// email is queued in the same transaction, so each admission sends one
// email whoever runs it, however often. It reports false when the user
// was not waiting (already admitted, or never on the list).
func Admit(ctx context.Context, pool *db.Pool, userID uuid.UUID, by string, now time.Time) (bool, error) {
	admitted := false
	err := db.InTx(ctx, pool, func(tx db.Tx) error {
		tag, err := tx.Exec(ctx, "update waitlist set admitted_at = $2, admitted_by = $3 where user_id = $1 and admitted_at is null", userID, now, by)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		admitted = true
		var email *string
		if err := tx.QueryRow(ctx, "select email from users where id = $1", userID).Scan(&email); err != nil {
			return err
		}
		id := store.NewID()
		if _, err := tx.Exec(ctx, `insert into events (id, project_id, user_id, ts, ts_second, kind, summary, source) values ($1, null, $2, $3, $4, $5, $6, 'api')`,
			id, userID, now, now.Unix(), Kind, Summary); err != nil {
			return err
		}
		// The admission email is transactional: it answers the user's own
		// request, so it goes out whatever notify_email says (I-269).
		if email != nil && *email != "" {
			if _, err := tx.Exec(ctx, "insert into events_outbox (event_id, channel) values ($1, 'email')", id); err != nil {
				return err
			}
		}
		return nil
	})
	return admitted, err
}

// Admitter lets the queue in as room appears.
type Admitter struct {
	Pool    *db.Pool
	Percent int
	M       *metrics.M
}

// Run admits waiting users oldest first while the fleet, with every
// admission inside AdmitWindow assumed to become a PendingClass guest,
// stays at or under Percent. It stops at the first user who does not fit:
// the queue is strictly in order. The caller holds db.LockWaitlist.
func (a *Admitter) Run(ctx context.Context, now time.Time) (int, error) {
	waiting, err := store.ListWaiting(ctx, a.Pool)
	if err != nil {
		return 0, err
	}
	n := 0
	defer func() {
		if a.M != nil {
			a.M.WaitlistWaiting.Set(float64(len(waiting) - n))
		}
	}()
	if a.Percent <= 0 || len(waiting) == 0 {
		// With the waitlist off, nobody new joins; those already waiting
		// are let in (the operator turned it off to open the doors).
		if a.Percent <= 0 {
			for _, w := range waiting {
				ok, err := Admit(ctx, a.Pool, w.UserID, "auto", now)
				if err != nil {
					return n, err
				}
				if ok {
					n++
					a.inc()
				}
			}
		}
		return n, nil
	}
	f, err := ReadFleet(ctx, a.Pool, now)
	if err != nil {
		return 0, err
	}
	for _, w := range waiting {
		if !f.Fits(a.Percent, scheduler.ClassRAM(PendingClass)) {
			break
		}
		ok, err := Admit(ctx, a.Pool, w.UserID, "auto", now)
		if err != nil {
			return n, err
		}
		if ok {
			n++
			f.Pending++
			a.inc()
		}
	}
	return n, nil
}

func (a *Admitter) inc() {
	if a.M != nil {
		a.M.WaitlistAdmittedTotal.Inc()
	}
}
