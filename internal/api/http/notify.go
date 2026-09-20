package httpapi

import (
	"net/http"
)

// unsubscribe serves the one-click link an email carries
// (13-notifications.md §5.6): no bearer token, no session, just the signed
// token naming the user. A worn or forged token gets a plain 400; there is
// nothing here worth an attacker probing for, so the message says why
// without echoing the token back.
func (s *Server) unsubscribe(w http.ResponseWriter, r *http.Request) error {
	if s.d.Unsub == nil {
		return errf("internal", "unsubscribe is not configured")
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		return errf("invalid", "token is required")
	}
	userID, err := s.d.Unsub.Verify(token)
	if err != nil {
		return errf("invalid", "this unsubscribe link is invalid or has expired")
	}
	if _, err := s.d.Pool.Exec(r.Context(), "update users set notify_email = false where id = $1", userID); err != nil {
		return err
	}
	// Mirror PATCH /me's behaviour (5.6's failure-mode table): a channel
	// turned off drops its already-queued rows rather than sending one
	// last batch to an inbox the user just asked to stop hearing from.
	if _, err := s.d.Pool.Exec(r.Context(),
		"delete from events_outbox where channel = 'email' and event_id in (select e.id from events e join projects p on p.id = e.project_id where p.user_id = $1)", userID); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("You have been unsubscribed from repose email notifications. You can turn them back on any time from the dashboard's notification settings.\n"))
	return nil
}
