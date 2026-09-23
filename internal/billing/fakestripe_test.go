package billing_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	stripe "github.com/stripe/stripe-go/v83"

	"github.com/heracraft/repose/internal/billing"
)

// fakeStripe is enough of Stripe's REST surface for the objects
// 09-billing.md §5.2 and §5.5 create: a customer, a SetupIntent, a
// subscription with the three metered prices, meter events, the portal
// session, the invoice list and the meter event summaries reconciliation
// reads back. It aggregates the meter events the way Stripe's `sum` meters
// do, so the fixture's invoice is computed from what was actually pushed
// rather than asserted against a constant.
//
// CI with a real STRIPE_SECRET_KEY runs the same fixture against Stripe
// test mode; this server is what makes the assertion run offline.
type fakeStripe struct {
	mu sync.Mutex
	// events[customer][meter] is the summed value, and seen holds the
	// identifiers so a duplicate is ignored the way Stripe's rolling
	// 24-hour uniqueness window ignores one.
	events map[string]map[string]int64
	seen   map[string]bool
	// calls counts requests by path, so a test can assert that a row with a
	// stored record id is never pushed again.
	calls map[string]int

	customers     []string
	subscriptions []string
	items         []map[string]any
	autoTax       bool
	invoices      []map[string]any
	// refuseTax makes a subscription with automatic tax fail the way
	// Stripe does for a customer it cannot locate; anchor is the
	// billing_cycle_anchor a new subscription reports.
	refuseTax bool
	anchor    int64
	// eventTimestamps holds every meter event's timestamp by identifier.
	eventTimestamps map[string]int64
	// pmCountry is the billing country the payment method reports, and
	// customerCountry the one the customer was last updated with.
	pmCountry       string
	customerCountry string
	// subList is every subscription created, as GET /v1/subscriptions
	// lists them, and lastSubKey the idempotency key of the last create.
	subList    []map[string]any
	lastSubKey string
	// lastCheckout is the form of the last Checkout session created.
	lastCheckout map[string]string

	// The bootstrap's objects (bootstrap_test.go).
	products      map[string]bool
	meters        []map[string]any
	prices        []map[string]any
	portalConfigs []map[string]any
	endpoints     []map[string]any
	taxStatus     string
	srv           *httptest.Server
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{events: map[string]map[string]int64{}, seen: map[string]bool{}, calls: map[string]int{},
		eventTimestamps: map[string]int64{}, products: map[string]bool{}, taxStatus: "pending"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/customers", f.customersHandler)
	mux.HandleFunc("/v1/customers/", f.customerUpdate)
	mux.HandleFunc("/v1/setup_intents", f.setupIntents)
	mux.HandleFunc("/v1/subscriptions", f.subscriptionsHandler)
	mux.HandleFunc("/v1/billing/meter_events", f.meterEvents)
	mux.HandleFunc("/v1/billing_portal/sessions", f.portal)
	mux.HandleFunc("/v1/invoices", f.invoiceList)
	mux.HandleFunc("/v1/billing/meters/", f.meterSummaries)
	mux.HandleFunc("/v1/payment_methods/", f.paymentMethod)
	mux.HandleFunc("/v1/checkout/sessions", f.checkoutSessions)
	f.bootstrapRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("fake stripe: unexpected %s %s", r.Method, r.URL.Path)
		// The t.Errorf above is the real report; this body only keeps the
		// Stripe client from blocking on an empty response.
		writeStripe(w, map[string]any{"error": map[string]any{"message": "no such route in the fake"}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// options builds the stripe-go client options that point the library at
// this server. stripe.NewClient takes a Backends whose URL is the base.
func (f *fakeStripe) options() []stripe.ClientOption {
	cfg := &stripe.BackendConfig{URL: stripe.String(f.srv.URL + "/v1")}
	b := stripe.GetBackendWithConfig(stripe.APIBackend, cfg)
	return []stripe.ClientOption{stripe.WithBackends(&stripe.Backends{API: b, Connect: b, Uploads: b, MeterEvents: b})}
}

func (f *fakeStripe) config() billing.Config {
	return billing.Config{
		SecretKey: "sk_test_fake", WebhookSecret: "whsec_fake", PortalReturnURL: "https://repose.test/billing",
		PriceCompute: "price_compute", PriceStorage: "price_storage", PriceEgress: "price_egress",
		MeterCompute: billing.DefaultMeterCompute, MeterStorage: billing.DefaultMeterStorage, MeterEgress: billing.DefaultMeterEgress,
		MeterIDCompute: "mtr_compute", MeterIDStorage: "mtr_storage", MeterIDEgress: "mtr_egress",
		Enforce: true, AutomaticTax: true,
	}
}

func writeStripe(w http.ResponseWriter, v map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (f *fakeStripe) count(path string) {
	f.mu.Lock()
	f.calls[path]++
	f.mu.Unlock()
}

func (f *fakeStripe) customersHandler(w http.ResponseWriter, r *http.Request) {
	f.count("customers")
	_ = r.ParseForm()
	f.mu.Lock()
	id := fmt.Sprintf("cus_fake%03d", len(f.customers)+1)
	f.customers = append(f.customers, id)
	f.mu.Unlock()
	writeStripe(w, map[string]any{"id": id, "object": "customer", "email": r.PostForm.Get("email")})
}

func (f *fakeStripe) customerUpdate(w http.ResponseWriter, r *http.Request) {
	f.count("customer_update")
	id := strings.TrimPrefix(r.URL.Path, "/v1/customers/")
	_ = r.ParseForm()
	if c := r.PostForm.Get("address[country]"); c != "" {
		f.mu.Lock()
		f.customerCountry = c
		f.mu.Unlock()
	}
	writeStripe(w, map[string]any{"id": id, "object": "customer"})
}

func (f *fakeStripe) setupIntents(w http.ResponseWriter, r *http.Request) {
	f.count("setup_intents")
	_ = r.ParseForm()
	writeStripe(w, map[string]any{"id": "seti_fake", "object": "setup_intent", "client_secret": "seti_fake_secret_abc",
		"customer": r.PostForm.Get("customer"), "usage": r.PostForm.Get("usage")})
}

func (f *fakeStripe) subscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		f.count("subscription_list")
		f.mu.Lock()
		data := append([]map[string]any{}, f.subList...)
		f.mu.Unlock()
		writeStripe(w, map[string]any{"object": "list", "has_more": false, "data": data})
		return
	}
	f.count("subscriptions")
	_ = r.ParseForm()
	f.mu.Lock()
	id := fmt.Sprintf("sub_fake%03d", len(f.subscriptions)+1)
	f.subscriptions = append(f.subscriptions, id)
	items := []map[string]any{}
	for i := 0; ; i++ {
		price := r.PostForm.Get(fmt.Sprintf("items[%d][price]", i))
		if price == "" {
			break
		}
		items = append(items, map[string]any{"id": fmt.Sprintf("si_fake%d", i), "object": "subscription_item", "price": map[string]any{"id": price, "object": "price"}})
	}
	f.items = items
	f.autoTax = r.PostForm.Get("automatic_tax[enabled]") == "true"
	refuse := f.refuseTax && f.autoTax
	f.mu.Unlock()
	if refuse {
		// What Stripe answers when it cannot locate the customer for tax.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"type": "invalid_request_error",
			"code": "customer_tax_location_invalid", "message": "The customer's location isn't recognized."}})
		return
	}
	sub := map[string]any{"id": id, "object": "subscription", "customer": r.PostForm.Get("customer"), "status": "active",
		"billing_cycle_anchor": f.anchor,
		"automatic_tax":        map[string]any{"enabled": r.PostForm.Get("automatic_tax[enabled]") == "true"},
		"items":                map[string]any{"object": "list", "has_more": false, "data": items}}
	f.mu.Lock()
	f.subList = append(f.subList, sub)
	f.lastSubKey = r.Header.Get("Idempotency-Key")
	f.mu.Unlock()
	writeStripe(w, sub)
}

func (f *fakeStripe) meterEvents(w http.ResponseWriter, r *http.Request) {
	f.count("meter_events")
	_ = r.ParseForm()
	name := r.PostForm.Get("event_name")
	id := r.PostForm.Get("identifier")
	customer := r.PostForm.Get("payload[stripe_customer_id]")
	value, _ := strconv.ParseInt(r.PostForm.Get("payload[value]"), 10, 64)
	f.mu.Lock()
	if !f.seen[id] {
		f.seen[id] = true
		f.eventTimestamps[id], _ = strconv.ParseInt(r.PostForm.Get("timestamp"), 10, 64)
		if f.events[customer] == nil {
			f.events[customer] = map[string]int64{}
		}
		f.events[customer][name] += value
	}
	f.mu.Unlock()
	writeStripe(w, map[string]any{"object": "billing.meter_event", "event_name": name, "identifier": id})
}

func (f *fakeStripe) portal(w http.ResponseWriter, r *http.Request) {
	f.count("portal")
	_ = r.ParseForm()
	writeStripe(w, map[string]any{"id": "bps_fake", "object": "billing_portal.session",
		"url": "https://billing.stripe.com/p/session/fake", "return_url": r.PostForm.Get("return_url")})
}

func (f *fakeStripe) invoiceList(w http.ResponseWriter, r *http.Request) {
	f.count("invoices")
	f.mu.Lock()
	data := append([]map[string]any{}, f.invoices...)
	f.mu.Unlock()
	writeStripe(w, map[string]any{"object": "list", "url": "/v1/invoices", "has_more": false, "data": data})
}

// meterSummaries answers /v1/billing/meters/{id}/event_summaries with one
// summary carrying the total pushed against that meter.
func (f *fakeStripe) meterSummaries(w http.ResponseWriter, r *http.Request) {
	f.count("meter_summaries")
	rest := strings.TrimPrefix(r.URL.Path, "/v1/billing/meters/")
	meterID := strings.TrimSuffix(rest, "/event_summaries")
	customer := r.URL.Query().Get("customer")
	name := map[string]string{
		"mtr_compute": billing.DefaultMeterCompute,
		"mtr_storage": billing.DefaultMeterStorage,
		"mtr_egress":  billing.DefaultMeterEgress,
	}[meterID]
	f.mu.Lock()
	v := f.events[customer][name]
	f.mu.Unlock()
	writeStripe(w, map[string]any{"object": "list", "has_more": false, "data": []map[string]any{
		{"object": "billing.meter_event_summary", "id": "mtrusg_fake", "meter": meterID, "aggregated_value": v},
	}})
}

// total is everything pushed for a customer, which is the invoice the
// fixture asserts.
func (f *fakeStripe) total(customer string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, v := range f.events[customer] {
		n += v
	}
	return n
}

func (f *fakeStripe) meter(customer, name string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.events[customer][name]
}

// automaticTax reports whether the subscription asked Stripe Tax to run
// (09-billing.md §5.9).
func (f *fakeStripe) automaticTax() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.autoTax
}

// subscriptionPrices lists the prices the subscription was created with.
func (f *fakeStripe) subscriptionPrices() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, it := range f.items {
		if p, ok := it["price"].(map[string]any); ok {
			if id, ok := p["id"].(string); ok {
				out = append(out, id)
			}
		}
	}
	return out
}

func (f *fakeStripe) callCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}
