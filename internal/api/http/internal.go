package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/ca/sshca"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

// internalRoute resolves <slug>.<handle> for the gateway.
func (s *Server) internalRoute(w http.ResponseWriter, r *http.Request) error {
	login := r.URL.Query().Get("login")
	i := strings.LastIndexByte(login, '.')
	if i <= 0 || i == len(login)-1 {
		return errf("invalid", "login must be <project>.<user>")
	}
	slug, handle := login[:i], login[i+1:]
	p, err := store.GetProjectByLogin(r.Context(), s.d.Pool, slug, handle)
	if err != nil {
		return err
	}
	out := map[string]any{"project_id": p.ID, "guest_ip": nil, "state": p.State, "principals": []string{p.ID.String()}, "host_unreachable": p.HostUnreachable}
	if p.GuestIP != nil {
		out["guest_ip"] = p.GuestIP.String()
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) internalRevoked(w http.ResponseWriter, r *http.Request) error {
	since := sinceParam(r)
	serials, err := s.d.CA.RevokedSince(r.Context(), since)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, serials)
	return nil
}

func (s *Server) internalCA(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, map[string]any{"user_ca_pub": s.d.CA.UserCAPub(), "host_ca_pub": s.d.CA.HostCAPub()})
	return nil
}

func (s *Server) internalSessions(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		ProjectID  string `json:"project_id"`
		Event      string `json:"event"`
		CertSerial int64  `json:"cert_serial"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	pid, err := uuid.Parse(body.ProjectID)
	if err != nil {
		return errf("invalid", "project_id is not valid")
	}
	if body.Event != "opened" && body.Event != "closed" {
		return errf("invalid", "event must be opened or closed")
	}
	opened := body.Event == "opened"
	closed := !opened
	s.sessions.update(pid, body.CertSerial, opened)
	ev := "session_open"
	if closed {
		ev = "session_close"
	}
	obs.Logger(r.Context(), s.d.Log).Info("gateway session", "event", ev, "project_id", pid.String(), "cert_serial", body.CertSerial)
	writeJSON(w, http.StatusOK, map[string]any{"open": s.sessions.Count(pid)})
	return nil
}

func (s *Server) internalHosts(w http.ResponseWriter, r *http.Request) error {
	hosts, err := store.ListHosts(r.Context(), s.d.Pool)
	if err != nil {
		return err
	}
	out := []map[string]any{}
	for _, h := range hosts {
		if h.RegisteredAt == nil || h.WGPubkey == nil {
			continue
		}
		row := map[string]any{"host_id": h.ID, "name": h.Name, "wg_pubkey": *h.WGPubkey, "state": h.State}
		if h.WGIP != nil {
			row["wg_ip"] = h.WGIP.String()
		}
		if h.GuestCIDR != nil {
			row["guest_cidr"] = h.GuestCIDR.String()
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (s *Server) internalGatewayCerts(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		PublicKey string `json:"public_key"`
		ProjectID string `json:"project_id"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	pub, err := sshca.ParsePublicKey(body.PublicKey)
	if err != nil {
		return errf("invalid", "public_key is not an OpenSSH public key")
	}
	pid, err := uuid.Parse(body.ProjectID)
	if err != nil {
		return errf("invalid", "project_id is not valid")
	}
	p, err := store.GetProject(r.Context(), s.d.Pool, pid)
	if err != nil {
		return err
	}
	if p.DestroyedAt != nil {
		return db.ErrNotFound
	}
	issued, err := s.d.CA.IssueGatewayCert(r.Context(), pub, p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"certificate": issued.Certificate, "expires_at": issued.ExpiresAt})
	return nil
}

func (s *Server) internalEvents(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		SourceIP string `json:"source_ip"`
		Agent    string `json:"agent"`
		Kind     string `json:"kind"`
		Summary  string `json:"summary"`
		Window   string `json:"window"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	id, err := s.d.Events.FromEdge(r.Context(), body.SourceIP, body.Agent, body.Kind, body.Summary)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errf("not_found", "no project at that address")
		}
		return errf("invalid", "%v", err)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"event_id": id, "ts": time.Now().UTC()})
	return nil
}
