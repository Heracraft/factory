package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/store"
)

// GET /projects/:id/ops/:op_id?wait=<duration>&seen=<version> (I-236): the
// op read as a long-poll. A CLI waiting on a start used to read the op
// (and the project, for its phase label) every 500 ms, so it learned that
// the guest was up on average a quarter of a second plus a round trip
// late. With wait, the answer comes as soon as the op's state or phase or
// its project's state differs from `seen` (or from what it was when the
// request came, without `seen`), or the op is finished, or the wait runs
// out. The server re-reads one small row every opWaitPoll and holds no
// database connection in between; hostd's results land in api-grpc, a
// different process, so there is no in-process event to wait on.
const (
	// OpWaitMax caps ?wait: under the proxies' idle timeouts, and short
	// enough that a rolling deploy's drain never waits on one.
	OpWaitMax = 20 * time.Second
	// opWaitPoll is how often a held request re-reads the op.
	opWaitPoll = 100 * time.Millisecond
	// OpWaitersPerUser and opWaitersTotal bound the held requests; past
	// either, the request is answered at once without the
	// Repose-Long-Poll header, and the client falls back to polling.
	OpWaitersPerUser = 4
	opWaitersTotal   = 1000
	// LongPollHeader marks an answer to a ?wait request that was held (or
	// needed no holding: the op had finished or already changed). A client
	// that sees it may ask again at once; without it (an older api, or the
	// waiter bound), it pauses between reads as before.
	LongPollHeader = "Repose-Long-Poll"
)

// opWaiters counts held op reads per user and in total.
type opWaiters struct {
	mu    sync.Mutex
	per   map[uuid.UUID]int
	total int
}

func (w *opWaiters) acquire(user uuid.UUID) (release func(), ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.per == nil {
		w.per = map[uuid.UUID]int{}
	}
	if w.per[user] >= OpWaitersPerUser || w.total >= opWaitersTotal {
		return nil, false
	}
	w.per[user]++
	w.total++
	return func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.total--
		if w.per[user]--; w.per[user] <= 0 {
			delete(w.per, user)
		}
	}, true
}

// parseWait reads ?wait as a Go duration ("25s", "1500ms") or whole
// seconds ("25"), capped at OpWaitMax; "" is no wait.
func parseWait(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		n, nerr := strconv.Atoi(v)
		if nerr != nil {
			return 0, errf("invalid", "wait: a duration such as 20s")
		}
		d = time.Duration(n) * time.Second
	}
	if d < 0 {
		return 0, errf("invalid", "wait: a duration such as 20s")
	}
	return min(d, OpWaitMax), nil
}

func opFinished(state string) bool { return state == "done" || state == "error" }

// opVersion is the opaque `version` of an op read: it changes when the
// op's state or step or its project's state does.
func opVersion(m store.OpMark) string {
	return fmt.Sprintf("%s.%d.%s", m.State, m.Step, m.ProjectState)
}

// opPhase is the name of the phase a running op is in, or "".
func opPhase(op *store.Op) string {
	if op.State != "running" {
		return ""
	}
	raw, _ := op.Params["phases"].([]any)
	if op.Step < 0 || op.Step >= len(raw) {
		return ""
	}
	s, _ := raw[op.Step].(string)
	return s
}

func (s *Server) getOp(w http.ResponseWriter, r *http.Request) error {
	p, op, err := s.userOpProject(r)
	if err != nil {
		return err
	}
	wait, err := parseWait(r.URL.Query().Get("wait"))
	if err != nil {
		return err
	}
	mark := store.OpMark{State: op.State, Step: op.Step, ProjectState: p.State}
	if wait > 0 {
		seen := r.URL.Query().Get("seen")
		switch {
		case opFinished(op.State) || (seen != "" && seen != opVersion(mark)):
			w.Header().Set(LongPollHeader, "1")
		default:
			release, ok := s.waiters.acquire(userFrom(r.Context()).ID)
			if !ok {
				break
			}
			held, changed, err := s.holdOp(r, op.ID, mark, wait)
			release()
			if err != nil {
				return err
			}
			if r.Context().Err() != nil {
				return nil // the client went away
			}
			w.Header().Set(LongPollHeader, "1")
			if changed {
				cur, err := store.GetOp(r.Context(), s.d.Pool, op.ID)
				if err != nil {
					return err
				}
				op, mark = cur, held
			}
		}
	}
	out := opJSON(r, op)
	out["version"] = opVersion(mark)
	out["project_state"] = mark.ProjectState
	if ph := opPhase(op); ph != "" {
		out["phase"] = ph
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// holdOp waits up to wait for the op's mark to differ from base, or the
// op to finish, or the api to begin draining (a rolling deploy: the
// request is answered, and the client asks the new container). It
// re-reads the mark every opWaitPoll without holding a connection.
func (s *Server) holdOp(r *http.Request, opID uuid.UUID, base store.OpMark, wait time.Duration) (store.OpMark, bool, error) {
	ctx := r.Context()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	tick := time.NewTicker(opWaitPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return base, false, nil
		case <-deadline.C:
			return base, false, nil
		case <-tick.C:
		}
		if s.draining.Load() {
			return base, false, nil
		}
		m, err := store.GetOpMark(ctx, s.d.Pool, opID)
		if err != nil {
			if ctx.Err() != nil {
				return base, false, nil
			}
			return base, false, err
		}
		if m != base || opFinished(m.State) {
			return m, true, nil
		}
	}
}
