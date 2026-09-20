package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/heracraft/repose/internal/api/buildlog"
	"github.com/heracraft/repose/internal/api/store"
)

// opLog streams BuildLog lines as server-sent events, every line in
// order, with ?since=<seq> catch-up and a final `done` event carrying the
// op's outcome.
func (s *Server) opLog(w http.ResponseWriter, r *http.Request) error {
	op, err := s.userOp(r)
	if err != nil {
		return err
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		return errf("internal", "streaming unsupported")
	}
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	ctx := r.Context()
	// Subscribe before the catch-up read so no line falls between them.
	ch, cancel := s.d.Logs.Subscribe(op.ID)
	defer cancel()
	last := since
	send := func(l buildlog.Line) {
		b, err := json.Marshal(l)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", l.Seq, b) // a gone client is noticed by ctx
		last = l.Seq
	}
	catchUp := func() error {
		for {
			lines, err := s.d.Logs.Read(ctx, op.ID, last, 1000)
			if err != nil {
				return err
			}
			for _, l := range lines {
				send(l)
			}
			flusher.Flush()
			if len(lines) < 1000 {
				return nil
			}
		}
	}
	if err := catchUp(); err != nil {
		return nil
	}
	finish := func(op *store.Op) {
		_ = catchUp()                                                            // best effort tail before done
		_, _ = fmt.Fprintf(w, "event: done\ndata: {\"state\":%q}\n\n", op.State) // same
		flusher.Flush()
	}
	if op.State == "done" || op.State == "error" {
		finish(op)
		return nil
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case l := <-ch:
			if l.Seq <= last {
				continue
			}
			if l.Seq > last+1 {
				if err := catchUp(); err != nil {
					return nil
				}
				if l.Seq <= last {
					continue
				}
			}
			send(l)
			flusher.Flush()
		case <-tick.C:
			cur, err := store.GetOp(ctx, s.d.Pool, op.ID)
			if err != nil {
				return nil
			}
			if cur.State == "done" || cur.State == "error" {
				finish(cur)
				return nil
			}
			_, _ = fmt.Fprint(w, ": keepalive\n\n") // same
			flusher.Flush()
		}
	}
}
