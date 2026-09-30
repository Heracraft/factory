package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/heracraft/repose/internal/billing"
)

// fakePaddle is the slice of Paddle's API the /billing routes reach:
// customers, transactions, subscriptions (patch, cancel, charge), portal
// sessions and invoice PDFs, with a signer for webhooks. The billing
// package has the full fake; this one is enough to drive the routes.
type fakePaddle struct {
	t      *testing.T
	srv    *httptest.Server
	cfg    billing.Config
	secret string
	mu     sync.Mutex
	seq    int
	bodies map[string]map[string]any
	subs   map[string]map[string]any
	txns   map[string]map[string]any
	custs  map[string]map[string]any
	evSeq  int
}

func newFakePaddle(t *testing.T) *fakePaddle {
	t.Helper()
	f := &fakePaddle{t: t, secret: "pdl_ntfset_route_test", bodies: map[string]map[string]any{}, subs: map[string]map[string]any{}, txns: map[string]map[string]any{}, custs: map[string]map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	f.cfg = billing.Config{APIKey: "pdl_sdbx_apikey_test", WebhookSecret: f.secret, ClientToken: "test_client_token",
		PriceSolo: "pri_solo_test", PricePlus: "pri_plus_test", PricePro: "pri_pro_test", ProductOverage: "pro_overage_test",
		DashboardURL: "https://repose.herakraft.co", BaseURL: f.srv.URL, Enforce: true}
	return f
}

func (f *fakePaddle) id(prefix string) string {
	f.seq++
	return fmt.Sprintf("%s_r%06d", prefix, f.seq)
}

func (f *fakePaddle) sign(body []byte) string { return billing.Sign(f.secret, time.Now(), body) }

func (f *fakePaddle) body(key string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[key]
}

func (f *fakePaddle) addSubscription(customer, price, status string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id("sub")
	f.subs[id] = map[string]any{"id": id, "status": status, "customer_id": customer, "next_billed_at": "2026-11-01T00:00:00Z",
		"current_billing_period": map[string]any{"starts_at": "2026-10-01T00:00:00Z", "ends_at": "2026-11-01T00:00:00Z"},
		"items":                  []any{map[string]any{"status": "active", "quantity": 1, "price": map[string]any{"id": price}}}}
	return id
}

func (f *fakePaddle) addTransaction(customer, sub, status string, total, tax int64) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.id("txn")
	f.txns[id] = map[string]any{"id": id, "status": status, "customer_id": customer, "subscription_id": sub, "currency_code": "USD", "invoice_number": "R-1", "created_at": "2026-10-01T00:00:00Z",
		"billing_period": map[string]any{"starts_at": "2026-10-01T00:00:00Z", "ends_at": "2026-11-01T00:00:00Z"},
		"details":        map[string]any{"totals": map[string]any{"subtotal": fmt.Sprint(total - tax), "tax": fmt.Sprint(tax), "total": fmt.Sprint(total), "grand_total": fmt.Sprint(total)}}}
	return id
}

func (f *fakePaddle) subData(id, userID, customer, price, status string) map[string]any {
	return map[string]any{"id": id, "status": status, "customer_id": customer, "next_billed_at": "2026-11-01T00:00:00Z",
		"current_billing_period": map[string]any{"starts_at": "2026-10-01T00:00:00Z", "ends_at": "2026-11-01T00:00:00Z"},
		"custom_data":            map[string]any{"user_id": userID},
		"items": []any{map[string]any{"status": "active", "quantity": 1, "price": map[string]any{"id": price},
			"trial_dates": map[string]any{"starts_at": "2026-10-01T00:00:00Z", "ends_at": "2026-10-08T00:00:00Z"}}}}
}

func (f *fakePaddle) event(kind string, data map[string]any) []byte {
	f.evSeq++
	b, _ := json.Marshal(map[string]any{"event_id": fmt.Sprintf("evt_r%06d", f.evSeq), "event_type": kind, "occurred_at": time.Now().UTC().Format(time.RFC3339), "data": data})
	return b
}

func (f *fakePaddle) write(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"request_id": "req_r"}})
}

func (f *fakePaddle) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.bodies[key] = body
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case r.Method == "GET" && r.URL.Path == "/customers":
		var out []any
		for _, c := range f.custs {
			if strings.EqualFold(r.URL.Query().Get("email"), c["email"].(string)) {
				out = append(out, c)
			}
		}
		f.write(w, 200, out)
	case r.Method == "POST" && r.URL.Path == "/customers":
		c := map[string]any{"id": f.id("ctm"), "email": body["email"], "status": "active"}
		f.custs[c["id"].(string)] = c
		f.write(w, 201, c)
	case r.Method == "POST" && len(parts) == 3 && parts[0] == "customers" && parts[2] == "portal-sessions":
		var subs []any
		if ids, ok := body["subscription_ids"].([]any); ok {
			for _, id := range ids {
				subs = append(subs, map[string]any{"id": id, "update_subscription_payment_method": "https://portal.fake/payment/" + id.(string)})
			}
		}
		f.write(w, 201, map[string]any{"id": f.id("pts"), "urls": map[string]any{"general": map[string]any{"overview": "https://portal.fake/overview/" + parts[1]}, "subscriptions": subs}})
	case r.Method == "POST" && r.URL.Path == "/transactions":
		tx := map[string]any{"id": f.id("txn"), "status": "ready", "customer_id": body["customer_id"], "custom_data": body["custom_data"]}
		f.txns[tx["id"].(string)] = tx
		f.write(w, 201, tx)
	case r.Method == "GET" && r.URL.Path == "/transactions":
		var out []any
		for _, tx := range f.txns {
			if tx["customer_id"] == r.URL.Query().Get("customer_id") && strings.Contains(r.URL.Query().Get("status"), tx["status"].(string)) {
				out = append(out, tx)
			}
		}
		f.write(w, 200, out)
	case r.Method == "GET" && len(parts) == 3 && parts[0] == "transactions" && parts[2] == "invoice":
		f.write(w, 200, map[string]any{"url": "https://invoices.fake/" + parts[1] + ".pdf"})
	case len(parts) >= 2 && parts[0] == "subscriptions":
		s, ok := f.subs[parts[1]]
		if !ok {
			w.WriteHeader(404)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "entity_not_found", "detail": "no such subscription"}})
			return
		}
		switch {
		case r.Method == "PATCH":
			if items, ok := body["items"].([]any); ok && len(items) > 0 {
				s["items"] = []any{map[string]any{"status": "active", "quantity": 1, "price": map[string]any{"id": items[0].(map[string]any)["price_id"]}}}
			}
			if sc, present := body["scheduled_change"]; present && sc == nil {
				s["scheduled_change"] = nil
			}
			f.write(w, 200, s)
		case r.Method == "POST" && len(parts) == 3 && parts[2] == "cancel":
			if body["effective_from"] == "immediately" {
				s["status"] = "canceled"
			} else {
				s["scheduled_change"] = map[string]any{"action": "cancel", "effective_at": "2026-11-01T00:00:00Z"}
			}
			f.write(w, 200, s)
		case r.Method == "POST" && len(parts) == 3 && parts[2] == "charge":
			out := map[string]any{"id": s["id"], "status": s["status"]}
			if body["effective_from"] == "immediately" {
				out["immediate_transaction"] = map[string]any{"id": f.id("txn")}
			}
			f.write(w, 200, out)
		default:
			f.write(w, 200, s)
		}
	default:
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found", "detail": key}})
	}
}
