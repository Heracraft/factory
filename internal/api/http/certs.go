package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/heracraft/repose/internal/api/ca"
	"github.com/heracraft/repose/internal/ca/sshca"
	"github.com/heracraft/repose/internal/obs"
)

func (s *Server) issueCert(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	var body struct {
		PublicKey  string   `json:"public_key"`
		ProjectIDs []string `json:"project_ids"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	pub, err := sshca.ParsePublicKey(body.PublicKey)
	if err != nil {
		return errf("invalid", "public_key is not an OpenSSH public key")
	}
	if len(body.ProjectIDs) == 0 || len(body.ProjectIDs) > 50 {
		return errf("invalid", "project_ids must list 1 to 50 projects")
	}
	ids := make([]uuid.UUID, 0, len(body.ProjectIDs))
	for _, p := range body.ProjectIDs {
		id, err := uuid.Parse(p)
		if err != nil {
			return errf("invalid", "project id %q is not valid", p)
		}
		ids = append(ids, id)
	}
	issued, err := s.d.CA.IssueUserCert(r.Context(), u, pub, ids)
	if err != nil {
		if errors.Is(err, ca.ErrNotOwner) {
			return errf("not_found", "project not found")
		}
		return err
	}
	s.d.Metrics.CertsIssuedTotal.Inc()
	obs.Logger(r.Context(), s.d.Log).Info("certificate issued", "event", "cert_issue", "cert_serial", issued.Serial, "projects", len(ids))
	writeJSON(w, http.StatusOK, map[string]any{
		"certificate": issued.Certificate, "serial": issued.Serial, "expires_at": issued.ExpiresAt,
		"gateway": map[string]any{"host": s.d.Gateway.Host, "port": s.d.Gateway.Port, "host_ca_pub": s.d.CA.HostCAPub()},
	})
	return nil
}

func (s *Server) revokeCert(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	var body struct {
		Serial *int64 `json:"serial"`
		All    bool   `json:"all"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Serial == nil && !body.All {
		return errf("invalid", "send serial or all")
	}
	if body.All {
		n, err := s.d.CA.RevokeAll(r.Context(), u.ID, "user:"+u.ID.String())
		if err != nil {
			return err
		}
		s.d.Metrics.CertsRevokedTotal.Add(float64(n))
		obs.Logger(r.Context(), s.d.Log).Info("certificates revoked", "event", "cert_revoke", "count", n)
		writeJSON(w, http.StatusOK, map[string]any{"revoked": n})
		return nil
	}
	if err := s.d.CA.Revoke(r.Context(), u.ID, *body.Serial); err != nil {
		return err
	}
	s.d.Metrics.CertsRevokedTotal.Inc()
	obs.Logger(r.Context(), s.d.Log).Info("certificate revoked", "event", "cert_revoke", "cert_serial", *body.Serial)
	writeJSON(w, http.StatusOK, map[string]any{"revoked": 1})
	return nil
}
