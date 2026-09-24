package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/heracraft/repose/internal/api/ops"
	"github.com/heracraft/repose/internal/api/store"
	"github.com/heracraft/repose/internal/db"
	"github.com/heracraft/repose/internal/obs"
)

func userJSON(u *store.User) map[string]any {
	return map[string]any{
		"id": u.ID, "handle": u.Handle, "email": u.Email, "github_login": u.GithubLogin, "tz": u.TZ, "created_at": u.CreatedAt,
		"billing": map[string]any{"status": u.BillingStatus, "trial_credit_cents": u.TrialCreditCents, "has_card": u.HasCard},
		"limits":  map[string]any{"projects": u.ProjectLimit, "xl": u.XLLimit},
		"notify":  map[string]any{"email": u.NotifyEmail, "ntfy_url": u.NtfyURL},
	}
}

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	// 09-billing.md §5.2: the Stripe customer is created at first GET /me.
	// A Stripe outage must not make the user's own profile unreadable, so a
	// failure is logged and the next /me tries again; nothing downstream
	// needs the customer until a card is added.
	if s.d.Customers != nil && (u.StripeCustomerID == nil || *u.StripeCustomerID == "") && u.BillingStatus != "exempt" {
		if _, err := s.d.Customers.EnsureCustomer(r.Context(), u.ID); err != nil {
			obs.Logger(r.Context(), s.d.Log).Warn("could not create the Stripe customer", "event", obs.EventStripeWebhook, "action", "customer_create", "err", err.Error())
		} else if fresh, err := store.GetUser(r.Context(), s.d.Pool, u.ID); err == nil {
			u = fresh
		}
	}
	writeJSON(w, http.StatusOK, userJSON(u))
	return nil
}

func (s *Server) patchMe(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	var body struct {
		TZ     *string `json:"tz"`
		Notify *struct {
			Email *bool `json:"email"`
			// Raw so an explicit null (clear, as interfaces/api.md says
			// and the CLI's `--ntfy none` sends) is told apart from an
			// absent key (leave alone); a *string made both nil, and
			// `--ntfy none` silently kept the old topic.
			NtfyURL json.RawMessage `json:"ntfy_url"`
		} `json:"notify"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.TZ != nil {
		if _, err := time.LoadLocation(*body.TZ); err != nil || strings.ContainsAny(*body.TZ, "\n\r") {
			return errf("invalid", "tz is not an IANA zone name")
		}
		if _, err := s.d.Pool.Exec(r.Context(), "update users set tz = $2 where id = $1", u.ID, *body.TZ); err != nil {
			return err
		}
	}
	if body.Notify != nil {
		if len(body.Notify.NtfyURL) > 0 {
			var ntfy *string
			if err := json.Unmarshal(body.Notify.NtfyURL, &ntfy); err != nil {
				return errf("invalid", "ntfy_url must be a string or null")
			}
			v := ""
			if ntfy != nil {
				v = strings.TrimSpace(*ntfy)
			}
			if v == "" {
				if _, err := s.d.Pool.Exec(r.Context(), "update users set ntfy_url = null where id = $1", u.ID); err != nil {
					return err
				}
				if _, err := s.d.Pool.Exec(r.Context(), "delete from events_outbox where channel = 'ntfy' and event_id in (select e.id from events e join projects p on p.id = e.project_id where p.user_id = $1)", u.ID); err != nil {
					return err
				}
			} else {
				pu, err := url.Parse(v)
				if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" {
					return errf("invalid", "ntfy_url must be an http(s) URL")
				}
				if _, err := s.d.Pool.Exec(r.Context(), "update users set ntfy_url = $2 where id = $1", u.ID, v); err != nil {
					return err
				}
			}
		}
		if body.Notify.Email != nil {
			if _, err := s.d.Pool.Exec(r.Context(), "update users set notify_email = $2 where id = $1", u.ID, *body.Notify.Email); err != nil {
				return err
			}
			if !*body.Notify.Email {
				if _, err := s.d.Pool.Exec(r.Context(), "delete from events_outbox where channel = 'email' and event_id in (select e.id from events e join projects p on p.id = e.project_id where p.user_id = $1)", u.ID); err != nil {
					return err
				}
			}
		}
	}
	fresh, err := store.GetUser(r.Context(), s.d.Pool, u.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, userJSON(fresh))
	return nil
}

// deleteMe begins cancellation: every live project is destroyed (a final
// snapshot kept 30 days) and the account is marked cancelled.
func (s *Server) deleteMe(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	ctx := r.Context()
	projects, err := store.ListUserProjects(ctx, s.d.Pool, u.ID)
	if err != nil {
		return err
	}
	var opIDs []string
	for i := range projects {
		p := &projects[i]
		pid := p.ID
		err := db.InTx(ctx, s.d.Pool, func(tx db.Tx) error {
			id, err := s.d.Engine.Enqueue(ctx, tx, ops.NewOp{Kind: ops.KindDestroy, ProjectID: &pid, Phases: ops.PlanDestroy(p)}, true)
			if err != nil {
				return err
			}
			opIDs = append(opIDs, id.String())
			return nil
		})
		if err != nil {
			return err
		}
	}
	if _, err := s.d.Pool.Exec(ctx, "update users set cancelled_at = coalesce(cancelled_at, now()) where id = $1", u.ID); err != nil {
		return err
	}
	if _, err := s.d.CA.RevokeAll(ctx, u.ID, "user:"+u.ID.String()); err != nil {
		return err
	}
	if _, err := store.Audit(ctx, s.d.Pool, "user:"+u.ID.String(), "account_cancel", u.ID.String(), map[string]any{"projects": len(projects)}); err != nil {
		return err
	}
	s.d.Engine.Kick()
	obs.Logger(ctx, s.d.Log).Info("account cancellation started", "event", "account_cancel", "projects", len(projects))
	writeJSON(w, http.StatusAccepted, map[string]any{"cancelled_at": time.Now().UTC(), "op_ids": opIDs, "retention_days": 30})
	return nil
}

func (s *Server) notifyTest(w http.ResponseWriter, r *http.Request) error {
	u := userFrom(r.Context())
	res := s.d.Outbox.Test(r.Context(), u)
	writeJSON(w, http.StatusOK, res)
	return nil
}
