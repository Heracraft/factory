// Package notify runs the notification outbox (05-control-plane-api.md
// §5.9, 13-notifications.md §5.5): undelivered rows are picked with
// `for update skip locked`, handed to the channel's sender, and marked
// delivered or retried on the documented schedule. Workstream 13 owns
// the senders' templates; the HTTP senders here carry the documented
// headers so delivery works from the first deploy.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
)

// Message is one notification to deliver.
type Message struct {
	EventID  uuid.UUID
	Kind     string
	Agent    string
	Project  string // slug
	Summary  string
	Email    string
	NtfyURL  string
	Dashboard string
}

// Sender delivers on one channel.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SenderFunc adapts a function.
type SenderFunc func(ctx context.Context, m Message) error

// Send implements Sender.
func (f SenderFunc) Send(ctx context.Context, m Message) error { return f(ctx, m) }

// Permanent wraps an error that must not be retried (a 4xx from ntfy).
type Permanent struct{ Err error }

func (p Permanent) Error() string { return p.Err.Error() }
func (p Permanent) Unwrap() error { return p.Err }

// backoff is the retry schedule; after the last attempt the row is
// marked failed (13-notifications.md §5.5, bounded by the 24 h in
// 05-control-plane-api.md §5.9).
var backoff = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 8 * time.Hour, 12 * time.Hour}

// Outbox is the worker.
type Outbox struct {
	pool      *db.Pool
	senders   map[string]Sender
	m         *metrics.M
	log       *slog.Logger
	Now       func() time.Time
	Dashboard string
	Interval  time.Duration
}

// New builds an outbox with the given channel senders.
func New(pool *db.Pool, senders map[string]Sender, m *metrics.M, log *slog.Logger) *Outbox {
	return &Outbox{pool: pool, senders: senders, m: m, log: log.With("component", "api"), Now: time.Now, Dashboard: "https://repose.herakraft.co", Interval: 2 * time.Second}
}

// Run polls until ctx ends, under the outbox advisory lock.
func (o *Outbox) Run(ctx context.Context) {
	for {
		release, ok, err := db.TryLock(ctx, o.pool, db.LockOutbox)
		if err != nil {
			o.log.Error("outbox lock", "event", "notify_fail", "err", err.Error())
		}
		if ok {
			t := time.NewTicker(o.Interval)
			for {
				if _, err := o.Once(ctx); err != nil && ctx.Err() == nil {
					o.log.Error("outbox pass", "event", "notify_fail", "err", err.Error())
				}
				select {
				case <-ctx.Done():
					t.Stop()
					release()
					return
				case <-t.C:
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

type row struct {
	eventID  uuid.UUID
	channel  string
	attempts int
	kind     string
	agent    *string
	summary  string
	slug     string
	email    *string
	ntfy     *string
	notifyEmail bool
}

// Once delivers every due row and returns how many it attempted.
func (o *Outbox) Once(ctx context.Context) (int, error) {
	now := o.Now()
	var rows []row
	err := db.InTx(ctx, o.pool, func(tx db.Tx) error {
		rs, err := tx.Query(ctx, `select o.event_id, o.channel, o.attempts, e.kind, e.agent, e.summary, p.slug, u.email, u.ntfy_url, u.notify_email
			from events_outbox o join events e on e.id = o.event_id join projects p on p.id = e.project_id join users u on u.id = p.user_id
			where o.next_at <= $1 order by o.next_at limit 100 for update of o skip locked`, now)
		if err != nil {
			return err
		}
		defer rs.Close()
		for rs.Next() {
			var r row
			if err := rs.Scan(&r.eventID, &r.channel, &r.attempts, &r.kind, &r.agent, &r.summary, &r.slug, &r.email, &r.ntfy, &r.notifyEmail); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		if err := rs.Err(); err != nil {
			return err
		}
		// Hold the rows out of other workers' reach for the delivery window.
		for _, r := range rows {
			if _, err := tx.Exec(ctx, "update events_outbox set next_at = $3 where event_id = $1 and channel = $2", r.eventID, r.channel, now.Add(time.Minute)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, r := range rows {
		o.deliver(ctx, r)
	}
	o.gauges(ctx)
	return len(rows), nil
}

func (o *Outbox) deliver(ctx context.Context, r row) {
	// A channel disabled since the row was queued is dropped.
	if (r.channel == "email" && (!r.notifyEmail || r.email == nil || *r.email == "")) || (r.channel == "ntfy" && (r.ntfy == nil || *r.ntfy == "")) {
		_, _ = o.pool.Exec(ctx, "delete from events_outbox where event_id = $1 and channel = $2", r.eventID, r.channel) // best effort; it is re-picked and dropped again otherwise
		return
	}
	s := o.senders[r.channel]
	if s == nil {
		o.mark(ctx, r, errors.New("no sender configured"), true)
		return
	}
	m := Message{EventID: r.eventID, Kind: r.kind, Project: r.slug, Summary: r.summary, Dashboard: o.Dashboard}
	if r.agent != nil {
		m.Agent = *r.agent
	}
	if r.email != nil {
		m.Email = *r.email
	}
	if r.ntfy != nil {
		m.NtfyURL = *r.ntfy
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := s.Send(sctx, m)
	cancel()
	var perm Permanent
	o.mark(ctx, r, err, errors.As(err, &perm))
}

func (o *Outbox) mark(ctx context.Context, r row, err error, permanent bool) {
	now := o.Now()
	if err == nil {
		o.m.NotifyTotal.WithLabelValues(r.channel, "ok").Inc()
		o.log.Info("notification sent", "event", "notify_send", "channel", r.channel, "kind", r.kind)
		_ = db.InTx(ctx, o.pool, func(tx db.Tx) error { // a failed mark re-sends once; the delivered key is idempotent
			if _, err := tx.Exec(ctx, "update events set delivered = delivered || jsonb_build_object($2::text, $3::text) where id = $1", r.eventID, r.channel, now.UTC().Format(time.RFC3339)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "delete from events_outbox where event_id = $1 and channel = $2", r.eventID, r.channel)
			return err
		})
		return
	}
	attempts := r.attempts + 1
	reason := err.Error()
	if len(reason) > 200 {
		reason = reason[:200]
	}
	if permanent || attempts > len(backoff) {
		o.m.NotifyTotal.WithLabelValues(r.channel, "failed").Inc()
		o.log.Warn("notification failed", "event", "notify_fail", "channel", r.channel, "kind", r.kind, "attempts", attempts)
		_ = db.InTx(ctx, o.pool, func(tx db.Tx) error { // see above
			if _, err := tx.Exec(ctx, "update events set delivered = delivered || jsonb_build_object($2::text, $3::text) where id = $1", r.eventID, r.channel, "failed: "+reason); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "delete from events_outbox where event_id = $1 and channel = $2", r.eventID, r.channel)
			return err
		})
		return
	}
	o.m.NotifyTotal.WithLabelValues(r.channel, "retry").Inc()
	next := now.Add(backoff[attempts-1])
	_, _ = o.pool.Exec(ctx, "update events_outbox set attempts = $3, next_at = $4, last_error = $5 where event_id = $1 and channel = $2", r.eventID, r.channel, attempts, next, reason) // best effort; the held row is re-picked in a minute
	_, _ = o.pool.Exec(ctx, "update events set delivered = delivered || jsonb_build_object($2::text, $3::text) where id = $1", r.eventID, r.channel, "error: "+reason)
}

func (o *Outbox) gauges(ctx context.Context) {
	var depth int
	var oldest *time.Time
	if err := o.pool.QueryRow(ctx, "select count(*), min(created_at) from events_outbox").Scan(&depth, &oldest); err == nil {
		o.m.OutboxDepth.Set(float64(depth))
		if oldest != nil {
			o.m.OutboxLagSeconds.Set(o.Now().Sub(*oldest).Seconds())
		} else {
			o.m.OutboxLagSeconds.Set(0)
		}
	}
}

// Title renders the one-line title of a message.
func Title(m Message) string {
	verb := map[string]string{"completed": "finished", "needs_input": "needs input", "error": "hit an error"}[m.Kind]
	if verb == "" {
		verb = strings.ReplaceAll(m.Kind, "_", " ")
	}
	if m.Agent != "" {
		return fmt.Sprintf("%s: %s %s", m.Project, m.Agent, verb)
	}
	return fmt.Sprintf("%s: %s", m.Project, verb)
}

// Email sends through Resend's HTTP API.
type Email struct {
	APIKey string
	From   string
	HTTP   *http.Client
	URL    string
}

// Send implements Sender.
func (e *Email) Send(ctx context.Context, m Message) error {
	if e.APIKey == "" {
		return errors.New("email: RESEND_API_KEY not set")
	}
	if m.Email == "" {
		return Permanent{errors.New("email: user has no address")}
	}
	from := e.From
	if from == "" {
		from = "repose <notify@repose.herakraft.co>"
	}
	url := e.URL
	if url == "" {
		url = "https://api.resend.com/emails"
	}
	title := Title(m)
	body := fmt.Sprintf("%s\n\n%s\n\nAttach with `repose attach --project %s` or open %s/projects.\n", title, m.Summary, m.Project, m.Dashboard)
	payload, err := json.Marshal(map[string]any{"from": from, "to": []string{m.Email}, "subject": "[repose] " + title, "text": body})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := e.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // status is all we need
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		return fmt.Errorf("email: status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return Permanent{fmt.Errorf("email: status %d", resp.StatusCode)}
	}
	return nil
}

// Ntfy posts to the user's ntfy URL.
type Ntfy struct {
	HTTP *http.Client
}

// Send implements Sender.
func (n *Ntfy) Send(ctx context.Context, m Message) error {
	if m.NtfyURL == "" {
		return Permanent{errors.New("ntfy: no url")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.NtfyURL, strings.NewReader(m.Summary))
	if err != nil {
		return Permanent{err}
	}
	req.Header.Set("Title", Title(m))
	prio, tag := "3", "white_check_mark"
	switch m.Kind {
	case "needs_input":
		prio, tag = "5", "question"
	case "error", "snapshot_failed", "base_update_failed", "billing_stopped":
		prio, tag = "4", "x"
	}
	req.Header.Set("Priority", prio)
	req.Header.Set("Tags", tag)
	req.Header.Set("Click", m.Dashboard+"/projects")
	client := n.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close() // status is all we need
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		return fmt.Errorf("ntfy: status %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return Permanent{fmt.Errorf("ntfy: status %d", resp.StatusCode)}
	}
	return nil
}

// Test sends a test message directly through every configured channel
// of the user and returns per-channel results (POST /me/notify-test).
func (o *Outbox) Test(ctx context.Context, u *store.User) map[string]string {
	out := map[string]string{}
	m := Message{Kind: "completed", Project: "repose", Summary: "This is a test from repose", Dashboard: o.Dashboard}
	if u.Email != nil {
		m.Email = *u.Email
	}
	if u.NtfyURL != nil {
		m.NtfyURL = *u.NtfyURL
	}
	if u.NotifyEmail && m.Email != "" {
		out["email"] = o.try(ctx, "email", m)
	}
	if m.NtfyURL != "" {
		out["ntfy"] = o.try(ctx, "ntfy", m)
	}
	return out
}

func (o *Outbox) try(ctx context.Context, channel string, m Message) string {
	s := o.senders[channel]
	if s == nil {
		return "error"
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.Send(sctx, m); err != nil {
		o.log.Warn("notify test failed", "event", "notify_fail", "channel", channel)
		return "error"
	}
	return "ok"
}
