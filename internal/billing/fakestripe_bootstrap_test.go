package billing_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// The rest of the fake: the payment method OnCardAttached reads, the
// hosted Checkout session, and the objects `stripe-bootstrap` finds or
// creates (product, meters, prices, portal configuration, webhook
// endpoint, tax settings). Lists answer everything they hold; Stripe's
// filters that matter to the bootstrap (lookup_keys, active) are applied.

func stripeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "invalid_request_error", "code": code, "message": msg}})
}

// arrayParam reads a form array however it is spelled: name[]=a or
// name[0]=a (stripe-go's form encoding).
func arrayParam(r *http.Request, name string) []string {
	var out []string
	for k, vs := range r.URL.Query() {
		if k == name+"[]" || strings.HasPrefix(k, name+"[") {
			out = append(out, vs...)
		}
	}
	return out
}

func list(data []map[string]any) map[string]any {
	if data == nil {
		data = []map[string]any{}
	}
	return map[string]any{"object": "list", "has_more": false, "data": data}
}

func (f *fakeStripe) paymentMethod(w http.ResponseWriter, r *http.Request) {
	f.count("payment_methods")
	id := strings.TrimPrefix(r.URL.Path, "/v1/payment_methods/")
	f.mu.Lock()
	country := f.pmCountry
	f.mu.Unlock()
	pm := map[string]any{"id": id, "object": "payment_method", "type": "card"}
	if country != "" {
		pm["billing_details"] = map[string]any{"address": map[string]any{"country": country, "line1": "1 Test St", "postal_code": "10001", "city": "Testville"}}
	}
	writeStripe(w, pm)
}

func (f *fakeStripe) checkoutSessions(w http.ResponseWriter, r *http.Request) {
	f.count("checkout_sessions")
	_ = r.ParseForm()
	f.mu.Lock()
	f.lastCheckout = map[string]string{}
	for k := range r.PostForm {
		f.lastCheckout[k] = r.PostForm.Get(k)
	}
	f.mu.Unlock()
	writeStripe(w, map[string]any{"id": "cs_test_fake", "object": "checkout.session", "mode": r.PostForm.Get("mode"),
		"url": "https://checkout.stripe.com/c/pay/cs_test_fake"})
}

func (f *fakeStripe) bootstrapRoutes(mux *http.ServeMux) {
	// A subscription update is only ever CycleNow's billing_cycle_anchor=now.
	mux.HandleFunc("/v1/subscriptions/", func(w http.ResponseWriter, r *http.Request) {
		f.count("subscription_update")
		_ = r.ParseForm()
		id := strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")
		f.mu.Lock()
		f.lastCheckout = map[string]string{"billing_cycle_anchor": r.PostForm.Get("billing_cycle_anchor"), "proration_behavior": r.PostForm.Get("proration_behavior")}
		anchor := f.anchor
		f.mu.Unlock()
		writeStripe(w, map[string]any{"id": id, "object": "subscription", "billing_cycle_anchor": anchor,
			"latest_invoice": map[string]any{"id": "in_cycle_now", "object": "invoice"}})
	})
	mux.HandleFunc("/v1/products/", func(w http.ResponseWriter, r *http.Request) {
		f.count("product_get")
		id := strings.TrimPrefix(r.URL.Path, "/v1/products/")
		f.mu.Lock()
		ok := f.products[id]
		f.mu.Unlock()
		if !ok {
			stripeErr(w, http.StatusNotFound, "resource_missing", "No such product: '"+id+"'")
			return
		}
		writeStripe(w, map[string]any{"id": id, "object": "product", "active": true})
	})
	mux.HandleFunc("/v1/products", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			f.count("product_get")
			var out []map[string]any
			f.mu.Lock()
			for _, id := range arrayParam(r, "ids") {
				if f.products[id] {
					out = append(out, map[string]any{"id": id, "object": "product", "active": true})
				}
			}
			f.mu.Unlock()
			writeStripe(w, list(out))
			return
		}
		f.count("product_create")
		_ = r.ParseForm()
		id := r.PostForm.Get("id")
		f.mu.Lock()
		f.products[id] = true
		f.mu.Unlock()
		writeStripe(w, map[string]any{"id": id, "object": "product", "active": true, "name": r.PostForm.Get("name")})
	})
	mux.HandleFunc("/v1/billing/meters", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodGet {
			writeStripe(w, list(f.meters))
			return
		}
		f.calls["meter_create"]++
		_ = r.ParseForm()
		m := map[string]any{"id": fmt.Sprintf("mtr_fake%d", len(f.meters)+1), "object": "billing.meter", "status": "active",
			"event_name":          r.PostForm.Get("event_name"),
			"default_aggregation": map[string]any{"formula": r.PostForm.Get("default_aggregation[formula]")},
			"customer_mapping":    map[string]any{"event_payload_key": r.PostForm.Get("customer_mapping[event_payload_key]"), "type": "by_id"},
			"value_settings":      map[string]any{"event_payload_key": r.PostForm.Get("value_settings[event_payload_key]")},
		}
		f.meters = append(f.meters, m)
		writeStripe(w, m)
	})
	mux.HandleFunc("/v1/prices", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodGet {
			keys := map[string]bool{}
			for _, k := range arrayParam(r, "lookup_keys") {
				keys[k] = true
			}
			var out []map[string]any
			for _, p := range f.prices {
				if len(keys) == 0 || keys[p["lookup_key"].(string)] {
					out = append(out, p)
				}
			}
			writeStripe(w, list(out))
			return
		}
		f.calls["price_create"]++
		_ = r.ParseForm()
		var amount int64
		_, _ = fmt.Sscan(r.PostForm.Get("unit_amount"), &amount)
		p := map[string]any{"id": fmt.Sprintf("price_fake%d", len(f.prices)+1), "object": "price", "active": true,
			"currency": r.PostForm.Get("currency"), "unit_amount": amount, "lookup_key": r.PostForm.Get("lookup_key"),
			"product": r.PostForm.Get("product"),
			"recurring": map[string]any{"interval": r.PostForm.Get("recurring[interval]"), "usage_type": r.PostForm.Get("recurring[usage_type]"),
				"meter": r.PostForm.Get("recurring[meter]")},
		}
		f.prices = append(f.prices, p)
		writeStripe(w, p)
	})
	mux.HandleFunc("/v1/billing_portal/configurations", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodGet {
			writeStripe(w, list(f.portalConfigs))
			return
		}
		f.calls["portal_config_create"]++
		_ = r.ParseForm()
		pc := map[string]any{"id": fmt.Sprintf("bpc_fake%d", len(f.portalConfigs)+1), "object": "billing_portal.configuration",
			"active": true, "metadata": map[string]string{"repose": r.PostForm.Get("metadata[repose]")},
			"cancel": r.PostForm.Get("features[subscription_cancel][enabled]")}
		f.portalConfigs = append(f.portalConfigs, pc)
		writeStripe(w, pc)
	})
	mux.HandleFunc("/v1/webhook_endpoints", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodGet {
			writeStripe(w, list(f.endpoints))
			return
		}
		f.calls["endpoint_create"]++
		_ = r.ParseForm()
		n := f.calls["endpoint_create"]
		ev := r.PostForm["enabled_events[]"]
		if len(ev) == 0 {
			for i := 0; ; i++ {
				v := r.PostForm.Get(fmt.Sprintf("enabled_events[%d]", i))
				if v == "" {
					break
				}
				ev = append(ev, v)
			}
		}
		e := map[string]any{"id": fmt.Sprintf("we_fake%d", n), "object": "webhook_endpoint", "url": r.PostForm.Get("url"),
			"api_version": r.PostForm.Get("api_version"), "enabled_events": ev, "status": "enabled"}
		f.endpoints = append(f.endpoints, e)
		out := map[string]any{}
		for k, v := range e {
			out[k] = v
		}
		out["secret"] = fmt.Sprintf("whsec_fake%d", n)
		writeStripe(w, out)
	})
	mux.HandleFunc("/v1/webhook_endpoints/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/webhook_endpoints/")
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, e := range f.endpoints {
			if e["id"] != id {
				continue
			}
			if r.Method == http.MethodDelete {
				f.calls["endpoint_delete"]++
				f.endpoints = append(f.endpoints[:i], f.endpoints[i+1:]...)
				writeStripe(w, map[string]any{"id": id, "object": "webhook_endpoint", "deleted": true})
				return
			}
			f.calls["endpoint_update"]++
			writeStripe(w, e)
			return
		}
		stripeErr(w, http.StatusNotFound, "resource_missing", "No such webhook endpoint")
	})
	mux.HandleFunc("/v1/tax/settings", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeStripe(w, map[string]any{"object": "tax.settings", "status": f.taxStatus})
	})
}
