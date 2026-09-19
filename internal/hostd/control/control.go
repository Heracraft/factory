// Package control is hostd's local operator API on a unix socket: what
// `hostd status`, `hostd guests`, `hostd snapshot-all`, `hostd drain` and
// `hostd reconcile` talk to while the daemon runs.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/heracraft/repose/internal/hostd/guest"
)

// StatusReply is GET /status.
type StatusReply struct {
	HostID    string         `json:"host_id"`
	Version   string         `json:"version"`
	Connected bool           `json:"stream_connected"`
	Draining  bool           `json:"draining"`
	Guests    map[string]int `json:"guests_by_state"`
	FreeMem   uint64         `json:"free_mem_bytes"`
	PoolFree  uint64         `json:"pool_free_bytes"`
	Uptime    string         `json:"uptime"`
}

// Backend is what the server needs from the daemon.
type Backend interface {
	Status(ctx context.Context) (*StatusReply, error)
	Guests(ctx context.Context) ([]guest.Status, error)
	SnapshotAll(reason string) ([]string, error)
	Drain(on bool) error
	Reconcile(ctx context.Context, rebuild bool) ([]string, error)
	ExportState(w io.Writer) error
}

// Serve listens on the socket until ctx ends.
func Serve(ctx context.Context, path string, b Backend) error {
	_ = os.Remove(path) // a stale socket from a crashed daemon; Listen reports anything else
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, v any, err error) {
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v) // a client that went away is not our problem
	}
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		s, err := b.Status(r.Context())
		writeJSON(w, s, err)
	})
	mux.HandleFunc("GET /guests", func(w http.ResponseWriter, r *http.Request) {
		g, err := b.Guests(r.Context())
		writeJSON(w, g, err)
	})
	mux.HandleFunc("POST /snapshot-all", func(w http.ResponseWriter, r *http.Request) {
		reason := r.URL.Query().Get("reason")
		if reason == "" {
			reason = "manual"
		}
		ids, err := b.SnapshotAll(reason)
		writeJSON(w, ids, err)
	})
	mux.HandleFunc("POST /drain", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, "ok", b.Drain(true))
	})
	mux.HandleFunc("POST /undrain", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, "ok", b.Drain(false))
	})
	mux.HandleFunc("POST /reconcile", func(w http.ResponseWriter, r *http.Request) {
		ids, err := b.Reconcile(r.Context(), r.URL.Query().Get("rebuild") == "1")
		writeJSON(w, ids, err)
	})
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := b.ExportState(w); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx) // shutting down; a slow client is cut off
		_ = os.Remove(path)    // best effort cleanup
	}()
	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Client talks to a running daemon.
type Client struct {
	Path string
	http *http.Client
}

// NewClient returns a client for the socket.
func NewClient(path string) *Client {
	return &Client{Path: path, http: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}}}
}

// ErrNotRunning is returned when the socket is absent.
var ErrNotRunning = errors.New("hostd is not running (no control socket)")

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://hostd"+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if _, serr := os.Stat(c.Path); serr != nil {
			return ErrNotRunning
		}
		return err
	}
	defer func() { _ = resp.Body.Close() }() // body read below
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("hostd: %s", string(body))
	}
	if out == nil {
		return nil
	}
	if w, ok := out.(io.Writer); ok {
		_, err = w.Write(body)
		return err
	}
	return json.Unmarshal(body, out)
}

func (c *Client) Status(ctx context.Context) (*StatusReply, error) {
	var s StatusReply
	return &s, c.do(ctx, http.MethodGet, "/status", &s)
}

func (c *Client) Guests(ctx context.Context) ([]guest.Status, error) {
	var g []guest.Status
	return g, c.do(ctx, http.MethodGet, "/guests", &g)
}

func (c *Client) SnapshotAll(ctx context.Context, reason string) ([]string, error) {
	var ids []string
	return ids, c.do(ctx, http.MethodPost, "/snapshot-all?reason="+reason, &ids)
}

func (c *Client) Drain(ctx context.Context, on bool) error {
	p := "/drain"
	if !on {
		p = "/undrain"
	}
	return c.do(ctx, http.MethodPost, p, nil)
}

func (c *Client) Reconcile(ctx context.Context, rebuild bool) ([]string, error) {
	q := ""
	if rebuild {
		q = "?rebuild=1"
	}
	var ids []string
	return ids, c.do(ctx, http.MethodPost, "/reconcile"+q, &ids)
}

func (c *Client) ExportState(ctx context.Context, w io.Writer) error {
	return c.do(ctx, http.MethodGet, "/state", w)
}
