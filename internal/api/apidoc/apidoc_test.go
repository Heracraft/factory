package apidoc

import "testing"

func TestLoadFindsEveryTable(t *testing.T) {
	routes, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"GET /me": false, "POST /certs": false, "GET /internal/route": false, "GET /projects/:id/ops/:op_id/log": false, "POST /me/notify-test": false, "GET /catalog": false}
	for _, r := range routes {
		if _, ok := want[r.Method+" "+r.Path]; ok {
			want[r.Method+" "+r.Path] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("route %s not parsed", k)
		}
	}
	if len(routes) < 35 {
		t.Fatalf("only %d routes parsed", len(routes))
	}
	if p := Pattern(Route{Method: "GET", Path: "/projects/:id/ops/:op_id"}); p != "GET /v1/projects/{id}/ops/{op_id}" {
		t.Fatal(p)
	}
}
