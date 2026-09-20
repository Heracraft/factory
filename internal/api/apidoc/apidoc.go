// Package apidoc parses the route tables of docs/interfaces/api.md so the
// router's contract test and the fake api can check themselves against
// the document rather than against each other.
package apidoc

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Route is one documented route.
type Route struct {
	Method string
	Path   string // as documented, e.g. /projects/:id/ops/:op_id/log
}

var rowRe = regexp.MustCompile("^\\| (GET|POST|PUT|PATCH|DELETE) \\| `([^`]+)` \\|")

// Parse reads routes from api.md text.
func Parse(text string) []Route {
	var out []Route
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		m := rowRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		path := m[2]
		if i := strings.Index(path, "?"); i >= 0 {
			path = path[:i]
		}
		out = append(out, Route{Method: m[1], Path: path})
	}
	return out
}

// Load reads docs/interfaces/api.md from the repository root.
func Load() ([]Route, error) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, "docs", "interfaces", "api.md"))
	if err != nil {
		return nil, err
	}
	return Parse(string(b)), nil
}

// Pattern converts a documented path to a Go 1.22 mux pattern under
// /v1: `:id` becomes `{id}`.
func Pattern(r Route) string {
	parts := strings.Split(r.Path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + p[1:] + "}"
		}
	}
	return r.Method + " /v1" + strings.Join(parts, "/")
}
