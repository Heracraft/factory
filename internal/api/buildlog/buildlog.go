// Package buildlog stores BuildLog lines per op in batches of 50 lines or
// 200 ms and publishes them on an in-process broadcast the SSE handler
// subscribes to (05-control-plane-api.md §5.4). Lines are scanned for
// the project's current secret values before storage and matching
// substrings replaced with [redacted] (docs/features/secrets.md).
package buildlog

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/db"
)

// Line is one stored line.
type Line struct {
	Seq  int64  `json:"seq"`
	Line string `json:"line"`
}

// Store batches, persists and broadcasts.
type Store struct {
	pool *db.Pool
	log  *slog.Logger

	mu       sync.Mutex
	pending  map[uuid.UUID][]Line
	redact   map[uuid.UUID][]string
	subs     map[uuid.UUID]map[chan Line]struct{}
	commands map[string]uuid.UUID // command_id -> op_id
	flushCh  chan struct{}
	batch    int
	interval time.Duration
}

// New makes a store.
func New(pool *db.Pool, log *slog.Logger) *Store {
	return &Store{pool: pool, log: log, pending: map[uuid.UUID][]Line{}, redact: map[uuid.UUID][]string{}, subs: map[uuid.UUID]map[chan Line]struct{}{},
		commands: map[string]uuid.UUID{}, flushCh: make(chan struct{}, 1), batch: 50, interval: 200 * time.Millisecond}
}

// Bind maps a command id to its op so incoming lines find their op.
func (s *Store) Bind(commandID string, opID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands[commandID] = opID
}

// Unbind forgets a command (op finished).
func (s *Store) Unbind(commandID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.commands, commandID)
}

// OpFor resolves a command id to its op; ok=false when unknown.
func (s *Store) OpFor(commandID string) (uuid.UUID, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.commands[commandID]
	return id, ok
}

// SetRedactions registers the strings that must never be stored for an op.
func (s *Store) SetRedactions(opID uuid.UUID, values []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []string
	for _, v := range values {
		if len(v) >= 4 {
			keep = append(keep, v)
		}
	}
	s.redact[opID] = keep
}

// ClearRedactions forgets an op's redaction set.
func (s *Store) ClearRedactions(opID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.redact, opID)
}

// Append queues a line; it is persisted by the next flush.
func (s *Store) Append(opID uuid.UUID, seq int64, line string) {
	s.mu.Lock()
	for _, v := range s.redact[opID] {
		line = strings.ReplaceAll(line, v, "[redacted]")
	}
	s.pending[opID] = append(s.pending[opID], Line{Seq: seq, Line: line})
	full := len(s.pending[opID]) >= s.batch
	s.mu.Unlock()
	if full {
		select {
		case s.flushCh <- struct{}{}:
		default:
		}
	}
}

// Run flushes on the interval or when a batch fills, until ctx ends.
func (s *Store) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.Flush(context.Background())
			return
		case <-t.C:
			s.Flush(ctx)
		case <-s.flushCh:
			s.Flush(ctx)
		}
	}
}

// Flush persists every pending line and publishes it.
func (s *Store) Flush(ctx context.Context) {
	s.mu.Lock()
	batch := s.pending
	s.pending = map[uuid.UUID][]Line{}
	s.mu.Unlock()
	for opID, lines := range batch {
		rows := make([][]any, 0, len(lines))
		for _, l := range lines {
			rows = append(rows, []any{opID, l.Seq, l.Line})
		}
		if err := db.InTx(ctx, s.pool, func(tx db.Tx) error {
			for _, r := range rows {
				if _, err := tx.Exec(ctx, "insert into build_logs (op_id, seq, line) values ($1, $2, $3) on conflict do nothing", r...); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			s.log.Error("build log flush failed", "event", "buildlog_flush_fail", "op_id", opID, "lines", len(lines), "err", err.Error())
			s.mu.Lock()
			s.pending[opID] = append(lines, s.pending[opID]...)
			s.mu.Unlock()
			continue
		}
		s.mu.Lock()
		subs := s.subs[opID]
		s.mu.Unlock()
		for ch := range subs {
			for _, l := range lines {
				select {
				case ch <- l:
				default: // a slow subscriber catches up from the table
				}
			}
		}
	}
}

// Subscribe returns a channel of lines for an op and a cancel function.
func (s *Store) Subscribe(opID uuid.UUID) (<-chan Line, func()) {
	ch := make(chan Line, 1024)
	s.mu.Lock()
	if s.subs[opID] == nil {
		s.subs[opID] = map[chan Line]struct{}{}
	}
	s.subs[opID][ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs[opID], ch)
		if len(s.subs[opID]) == 0 {
			delete(s.subs, opID)
		}
		s.mu.Unlock()
	}
}

// Read returns stored lines with seq > since, in order.
func (s *Store) Read(ctx context.Context, opID uuid.UUID, since int64, limit int) ([]Line, error) {
	if limit <= 0 {
		limit = 10000
	}
	rows, err := s.pool.Query(ctx, "select seq, line from build_logs where op_id = $1 and seq > $2 order by seq limit $3", opID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Line{}
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.Seq, &l.Line); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Trim keeps the logs of the newest keep ops per project (docs/ops/
// OBSERVABILITY.md retention) and returns rows deleted.
func (s *Store) Trim(ctx context.Context, keep int) (int64, error) {
	tag, err := s.pool.Exec(ctx, `delete from build_logs where op_id in (
		select id from (select id, row_number() over (partition by project_id order by created_at desc) as rn from ops where kind in ('build','create')) o where rn > $1)`, keep)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
