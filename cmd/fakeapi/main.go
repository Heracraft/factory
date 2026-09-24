// Command fakeapi runs internal/fakes/api as a standalone process on a
// random localhost port, for the dashboard's Playwright suite to spawn
// (docs/workstreams/08-dashboard.md §7: "Playwright end to end against
// internal/fakes/api (Go, run as a test fixture on a port)"). A second,
// separate listener exposes the fake's error switch over HTTP, since
// internal/fakes/api.Fake.Fail/FailNext/Unfail are Go-only methods and a
// Playwright spec has no other way to call them.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heracraft/repose/internal/fakes/api"
)

func main() {
	billing := flag.Bool("billing", false, "enable the billing routes instead of 503 billing_disabled")
	flag.Parse()

	f := api.New(api.Options{Billing: *billing})
	defer f.Close()

	admin, err := newAdminServer(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fakeapi:", err)
		os.Exit(1)
	}
	defer func() { _ = admin.Close() }()

	// The two lines of stdout this process ever prints, so a harness's
	// line-reader never has to guess which line is which.
	fmt.Printf("FAKEAPI_URL=%s/v1\n", f.URL())
	fmt.Printf("FAKEAPI_ADMIN_URL=%s\n", admin.URL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}

type adminServer struct {
	*http.Server
	URL string
}

func (a *adminServer) Close() error { return a.Server.Close() }

// newAdminServer starts the switch on 127.0.0.1:0. Routes:
//
//	POST /fail       {"method","path","code"} -> f.Fail
//	POST /fail-next  {"method","path","code"} -> f.FailNext
//	POST /unfail     {"method","path"}        -> f.Unfail
//	POST /billing    {"mode": off|card|nocard} -> f.SetBilling
//	POST /question   {"project_id","agent","text","options","timeout_s"} -> f.AddQuestion
func newAdminServer(f *api.Fake) (*adminServer, error) {
	type req struct{ Method, Path, Code string }
	decode := func(w http.ResponseWriter, r *http.Request) (req, bool) {
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return req{}, false
		}
		return body, true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /fail", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := decode(w, r); ok {
			f.Fail(body.Method+" "+body.Path, body.Code)
		}
	})
	mux.HandleFunc("POST /billing", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Mode string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		switch body.Mode {
		case "off":
			f.SetBilling(api.BillingOff)
		case "card":
			f.SetBilling(api.BillingCard)
		case "nocard":
			f.SetBilling(api.BillingNoCard)
		default:
			http.Error(w, "mode is off, card or nocard", http.StatusBadRequest)
		}
	})
	mux.HandleFunc("POST /question", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ProjectID string   `json:"project_id"`
			Agent     string   `json:"agent"`
			Text      string   `json:"text"`
			Options   []string `json:"options"`
			TimeoutS  int      `json:"timeout_s"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q, err := f.AddQuestion(body.ProjectID, body.Agent, body.Text, body.Options, time.Duration(body.TimeoutS)*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(q)
	})
	mux.HandleFunc("POST /fail-next", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := decode(w, r); ok {
			f.FailNext(body.Method, body.Path, body.Code)
		}
	})
	mux.HandleFunc("POST /unfail", func(w http.ResponseWriter, r *http.Request) {
		if body, ok := decode(w, r); ok {
			f.Unfail(body.Method + " " + body.Path)
		}
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("fakeapi: admin listener: %w", err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	return &adminServer{Server: srv, URL: "http://" + ln.Addr().String()}, nil
}
