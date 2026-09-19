// Command repose-hook is what an agent's hook configuration calls. It reads
// the agent's own hook payload, maps it to {agent, kind, summary}, and POSTs
// it to guestd's hook socket.
//
// It always exits 0. A hook that fails must never block an agent, because a
// blocked agent is a silently wasted night (docs/features/agents.md).
//
// Workstream: docs/workstreams/04-guestd.md.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/heracraft/repose/internal/guestd/hooks"
)

var version = "dev" // set by -ldflags at release

// DefaultSocket is the hook socket of docs/interfaces/guest-conventions.md.
const DefaultSocket = "/run/repose/hooks.sock"

// Timeout bounds the POST. The agent is waiting on this process.
const Timeout = 3 * time.Second

func main() {
	// Whatever happens below, the exit code is zero.
	if msg := run(); msg != "" {
		fmt.Fprintln(os.Stderr, "repose-hook:", msg)
	}
}

func run() string {
	fs := flag.NewFlagSet("repose-hook", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		agent   = fs.String("agent", os.Getenv("REPOSE_AGENT"), "which agent is reporting")
		socket  = fs.String("socket", socketDefault(), "guestd hook socket")
		window  = fs.String("window", os.Getenv("REPOSE_AGENT_WINDOW"), "tmux window name, if the wrapper knows it")
		showVer = fs.Bool("version", false, "print the version and exit")
	)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err.Error()
	}
	if *showVer {
		fmt.Println("repose-hook", version)
		return ""
	}
	if *agent == "" {
		return "no agent given; pass --agent or set REPOSE_AGENT"
	}

	payload, err := readPayload(fs.Args())
	if err != nil {
		return err.Error()
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return "empty payload"
	}

	p, err := hooks.Map(*agent, payload)
	if err != nil {
		if errors.Is(err, hooks.ErrNoEvent) {
			return ""
		}
		return err.Error()
	}
	if *window != "" {
		p.Window = *window
	}

	if err := post(*socket, p); err != nil {
		return err.Error()
	}
	return ""
}

func socketDefault() string {
	if s := os.Getenv("REPOSE_HOOK_SOCKET"); s != "" {
		return s
	}
	return DefaultSocket
}

// readPayload takes the JSON from the first argument when there is one (Codex
// CLI's notify passes it that way) and from stdin otherwise (Claude Code's
// hooks do).
func readPayload(args []string) ([]byte, error) {
	if len(args) > 0 && args[0] != "" && args[0] != "-" {
		return []byte(args[0]), nil
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read the hook payload: %w", err)
	}
	return b, nil
}

func post(socket string, p hooks.Payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode the event: %w", err)
	}
	client := &http.Client{
		Timeout: Timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://guestd/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post to the hook socket: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // response is drained below
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("the hook socket answered %d", resp.StatusCode)
	}
	return nil
}
