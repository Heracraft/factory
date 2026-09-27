package httpapi

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/api/waitlist"
)

// waitlistJSON is the `waitlist` object of GET /me and GET /billing.
func waitlistJSON(e *store.WaitlistEntry) map[string]any {
	return map[string]any{"position": e.Position, "joined_at": e.JoinedAt, "invited_at": e.InvitedAt, "hold_until": e.HoldUntil}
}

// billingWaitlist is POST /billing/waitlist: join the seats waitlist
// without a checkout (DECISIONS I-290). Idempotent: a second call answers
// the same place.
func (s *Server) billingWaitlist(w http.ResponseWriter, r *http.Request) error {
	if s.d.Seats == nil {
		return errors.New("seats not configured")
	}
	u := userFrom(r.Context())
	p, err := s.d.Seats.Join(r.Context(), u.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"position": p.Position, "joined_at": p.JoinedAt})
	return nil
}

// seatsCacheTTL is how long GET /public/seats serves the last count: the
// landing page asks on every visit and the number moves by the minute.
const seatsCacheTTL = time.Minute

type seatsCache struct {
	mu    sync.Mutex
	at    time.Time
	count waitlist.Count
}

// publicSeats is GET /public/seats: `{total, free, waiting}` for the
// landing page, no token, cached a minute in memory.
func (s *Server) publicSeats(w http.ResponseWriter, r *http.Request) error {
	if s.d.Seats == nil {
		return errors.New("seats not configured")
	}
	s.seats.mu.Lock()
	defer s.seats.mu.Unlock()
	now := time.Now()
	if s.seats.at.IsZero() || now.Sub(s.seats.at) >= seatsCacheTTL {
		c, err := s.d.Seats.Count(r.Context())
		if err != nil {
			return err
		}
		s.seats.count, s.seats.at = c, now
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{"total": s.seats.count.Total, "free": s.seats.count.Free, "waiting": s.seats.count.Waiting})
	return nil
}
