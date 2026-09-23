// Package meter ingests Samples into meter_samples and proc_samples and
// rolls them up hourly into usage_hours (05-control-plane-api.md §5.4,
// §5.10). Samples are append-only and never read on a request path; the
// rollup reads them once.
package meter

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/heracraft/repose/internal/api/metrics"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// Ingest writes samples.
type Ingest struct {
	pool *db.Pool
	m    *metrics.M
	log  *slog.Logger

	mu      sync.Mutex
	guests  map[uuid.UUID]guestEntry
	months  map[string]bool
	nowFunc func() time.Time
}

type guestEntry struct {
	project uuid.UUID
	seen    time.Time
}

// New builds an ingest.
func New(pool *db.Pool, m *metrics.M, log *slog.Logger) *Ingest {
	return &Ingest{pool: pool, m: m, log: log.With("component", "api"), guests: map[uuid.UUID]guestEntry{}, months: map[string]bool{}, nowFunc: time.Now}
}

func (i *Ingest) project(ctx context.Context, guestID string) (uuid.UUID, bool) {
	gid, err := uuid.Parse(guestID)
	if err != nil {
		return uuid.Nil, false
	}
	i.mu.Lock()
	e, ok := i.guests[gid]
	i.mu.Unlock()
	if ok && i.nowFunc().Sub(e.seen) < 10*time.Minute {
		return e.project, true
	}
	p, err := store.GetProjectByGuest(ctx, i.pool, gid)
	if err != nil {
		return uuid.Nil, false
	}
	i.mu.Lock()
	i.guests[gid] = guestEntry{project: p.ID, seen: i.nowFunc()}
	i.mu.Unlock()
	return p.ID, true
}

func (i *Ingest) ensureMonth(ctx context.Context, ts time.Time) error {
	key := ts.UTC().Format("2006-01")
	i.mu.Lock()
	ok := i.months[key]
	i.mu.Unlock()
	if ok {
		return nil
	}
	if err := db.EnsurePartitions(ctx, i.pool, ts); err != nil {
		return err
	}
	i.mu.Lock()
	i.months[key] = true
	i.mu.Unlock()
	return nil
}

// OnSamples stores one Samples message. Rows are inserted with ON
// CONFLICT DO NOTHING so a message hostd re-sends after a reconnect is
// harmless.
func (i *Ingest) OnSamples(ctx context.Context, hostID uuid.UUID, s *hostdv1.Samples) {
	ts := time.Unix(s.Ts, 0).UTC()
	if s.Ts == 0 {
		ts = i.nowFunc().UTC()
	}
	if err := i.ensureMonth(ctx, ts); err != nil {
		i.log.Error("sample partition", "event", "samples_fail", "err", err.Error())
		return
	}
	batch := &pgx.Batch{}
	n := 0
	for _, g := range s.Guests {
		pid, ok := i.project(ctx, g.GuestId)
		if !ok {
			continue
		}
		sig := g.Signals
		if sig == nil {
			sig = &hostdv1.GuestSignals{}
		}
		agents := []map[string]string{}
		for _, a := range sig.Agents {
			agents = append(agents, map[string]string{"agent": a.Agent, "window": a.TmuxWindow, "state": a.State})
		}
		aj, _ := json.Marshal(agents)
		listening := make([]Listening, 0, len(sig.Listening))
		for _, l := range sig.Listening {
			listening = append(listening, Listening{Port: int(l.Port), Comm: l.Comm, AgeSeconds: int64(l.AgeSeconds), RSSBytes: int64(l.RssBytes)})
		}
		lj, _ := json.Marshal(listening)
		batch.Queue(`insert into meter_samples (ts, project_id, host_id, state, class, cpu_ns, mem_rss, net_tx, net_rx, disk_alloc, disk_used, ssh_sessions, tmux_clients, agents, docker_containers, guestd_ok, listening)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) on conflict do nothing`,
			ts, pid, hostID, g.State, g.Class, int64(g.CpuNsDelta), int64(g.MemRssBytes), int64(g.NetTxBytesDelta), int64(g.NetRxBytesDelta), int64(g.DiskAllocBytes), int64(g.DiskUsedBytes),
			int32(sig.SshSessions), int32(sig.TmuxClients), aj, int32(sig.DockerContainers), sig.GuestdOk, lj)
		n++
		seen := map[string]bool{}
		for _, p := range g.Procs {
			if p.Comm == "" || seen[p.Comm] {
				continue
			}
			seen[p.Comm] = true
			batch.Queue("insert into proc_samples (ts, project_id, comm, cpu_ns, rss) values ($1,$2,$3,$4,$5) on conflict do nothing", ts, pid, p.Comm, int64(p.CpuNsDelta), int64(p.RssBytes))
		}
	}
	if n == 0 {
		return
	}
	br := i.pool.SendBatch(ctx, batch)
	if err := br.Close(); err != nil {
		i.log.Error("sample insert", "event", "samples_fail", "host_id", hostID.String(), "err", err.Error())
	}
}

// Listening is one of the guest's listening processes (I-200), as
// stored and as served in Project.signals.listening.
type Listening struct {
	Port       int    `json:"port"`
	Comm       string `json:"comm,omitempty"`
	AgeSeconds int64  `json:"age_seconds,omitempty"`
	RSSBytes   int64  `json:"rss_bytes,omitempty"`
}

// Latest is the newest sample of a project, for GET /projects/:id.
type Latest struct {
	// Listening is nil when the sample carried none (a hostd or guestd
	// older than I-200), empty when the guest listens on nothing.
	Listening        []Listening
	TS               time.Time
	State            string
	SSHSessions      int
	TmuxClients      int
	Agents           []map[string]string
	DockerContainers int
	DiskUsed         int64
	GuestdOK         bool
}

// LatestSample reads the newest sample; ok=false when there is none.
func LatestSample(ctx context.Context, q store.Querier, projectID uuid.UUID) (*Latest, bool, error) {
	var l Latest
	var agents, listening []byte
	err := q.QueryRow(ctx, "select ts, state, ssh_sessions, tmux_clients, agents, docker_containers, disk_used, guestd_ok, listening from meter_samples where project_id = $1 order by ts desc limit 1", projectID).
		Scan(&l.TS, &l.State, &l.SSHSessions, &l.TmuxClients, &agents, &l.DockerContainers, &l.DiskUsed, &l.GuestdOK, &listening)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	_ = json.Unmarshal(agents, &l.Agents) // stored by us; malformed means empty
	if listening != nil {
		_ = json.Unmarshal(listening, &l.Listening)
	}
	return &l, true, nil
}
