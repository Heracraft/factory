package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// REPOSE_TIMING=1 prints one line per phase, api call and ssh to stderr
// (DECISIONS I-223): "repose-timing +<ms since start> <what> <ms>". It is
// for measuring where `repose run` and `repose attach` spend their time
// (ops/dev/startup-bench.sh reads it). Lines carry api routes with ids
// replaced by ":id", ssh step names and byte counts, never a remote
// command, a path or a token.

var timingStart = time.Now()

var timingOut = func() io.Writer {
	if os.Getenv(envTiming) == "1" {
		return os.Stderr
	}
	return nil
}()

var timingMu sync.Mutex

func timingEnabled() bool { return timingOut != nil }

// timingf writes one timing line when REPOSE_TIMING=1.
func timingf(format string, args ...any) {
	if timingOut == nil {
		return
	}
	timingMu.Lock()
	defer timingMu.Unlock()
	_, _ = fmt.Fprintf(timingOut, "repose-timing +%dms %s\n", time.Since(timingStart).Milliseconds(), fmt.Sprintf(format, args...))
}

// timeSpan returns a func that, called at the span's end, writes "<what>
// <ms>ms". Use as `defer timeSpan("connect")()`.
func timeSpan(what string) func() {
	if timingOut == nil {
		return func() {}
	}
	t := time.Now()
	return func() { timingf("%s %dms", what, time.Since(t).Milliseconds()) }
}

var idInPath = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|op_[A-Za-z0-9]+`)

// timingTransport times each HTTP round trip (api and the login
// provider's token refresh).
type timingTransport struct{ next http.RoundTripper }

func (t timingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.next.RoundTrip(r)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	path := idInPath.ReplaceAllString(r.URL.Path, ":id")
	host := r.URL.Host
	if i := strings.IndexByte(host, '.'); i > 0 {
		host = host[:i]
	}
	timingf("http %s %s%s %d %dms", r.Method, host, path, status, time.Since(start).Milliseconds())
	return resp, err
}

// withTiming wraps c's transport when timing is on.
func withTiming(c *http.Client) *http.Client {
	if timingOut == nil {
		return c
	}
	next := c.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	c.Transport = timingTransport{next}
	return c
}

// sshLabel names a remote command for a timing line without printing it:
// the first word of a one-liner ("true", "tmux"), or "script" and its
// size.
func sshLabel(remoteCmd string) string {
	if strings.ContainsAny(remoteCmd, "\n;|&") || len(remoteCmd) > 40 {
		return fmt.Sprintf("script(%dB)", len(remoteCmd))
	}
	w, _, _ := strings.Cut(strings.TrimSpace(remoteCmd), " ")
	return w
}
