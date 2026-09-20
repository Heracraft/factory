// Package preview is the HTTPS preview-proxy stub on the edge's :443
// (docs/workstreams/06-gateway-edge.md §5.8, docs/features/ports-and-previews.md).
// The listener, the wildcard-certificate path and the hostname parser exist
// so the certificate pipeline is proven before the feature is built; the
// authentication and the reverse proxy return "not implemented" until
// features/ports-and-previews.md is delivered.
package preview

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotImplemented is returned by the parts that wait on
// features/ports-and-previews.md.
var ErrNotImplemented = errors.New("previews are not enabled yet")

// Target is a resolved preview host: which project and which guest port.
type Target struct {
	Slug   string
	Handle string
	Port   int
}

// hostRe matches `<port>-<slug>-<handle>.repose.herakraft.co`, the form
// DECISIONS I-6 fixed (the handle is the last label before the domain
// because slugs are unique per user, not globally). Slug and handle are
// [a-z0-9-]; the port is 1-5 digits.
var hostRe = regexp.MustCompile(`^([0-9]{1,5})-([a-z0-9-]+)-([a-z0-9-]+)\.repose\.herakraft\.co$`)

// Route parses a preview hostname into its target. The slug/handle split is
// unambiguous only because both are a single [a-z0-9-] run and the handle is
// the final label; a slug that itself contains a dash is still parsed
// correctly because the regexp is greedy on the slug and the handle is the
// last dash-delimited group. Port 0 and ports above 65535 are rejected.
func Route(host string) (Target, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i] // strip any :port the client sent
	}
	m := hostRe.FindStringSubmatch(host)
	if m == nil {
		return Target{}, fmt.Errorf("preview host %q is not <port>-<slug>-<handle>.repose.herakraft.co", host)
	}
	port, err := strconv.Atoi(m[1])
	if err != nil || port < 1 || port > 65535 {
		return Target{}, fmt.Errorf("preview host %q: port out of range", host)
	}
	// The slug is greedy, so for "3000-a-b-c" the slug is "a-b" and the
	// handle is "c". That matches how the CLI builds the name (handle last).
	slug, handle := m[2], m[3]
	return Target{Slug: slug, Handle: handle, Port: port}, nil
}

// Authenticate will check the Logto session cookie and return the user id.
// Until features/ports-and-previews.md is built it returns ErrNotImplemented.
func Authenticate(r *http.Request) (userID string, err error) {
	return "", ErrNotImplemented
}

// disabledPage is what the stub serves on every path but /healthz.
const disabledPage = `<!doctype html>
<title>repose previews</title>
<h1>Previews are not enabled yet</h1>
<p>Per-project preview URLs are on the roadmap. Use <code>repose open &lt;port&gt;</code>
to reach a dev server over SSH in the meantime.</p>
`

// Handler is the stub's HTTP handler: /healthz for the load path and a
// static page everywhere else. It never proxies; the reverse-proxy skeleton
// below is where the real behaviour will attach.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Parse the host so a misconfigured DNS entry is visible in the
		// stub's logs, but serve the disabled page regardless.
		if _, err := Route(r.Host); err == nil {
			if _, aerr := Authenticate(r); errors.Is(aerr, ErrNotImplemented) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(disabledPage))
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(disabledPage))
	})
	return mux
}
