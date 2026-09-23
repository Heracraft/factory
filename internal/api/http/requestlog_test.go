package httpapi_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// The request log line names the matched route, the user and which client
// sent it, so a destroy can be traced to the CLI or the dashboard. Before
// this every line said route "unmatched" with no user_id: the mux sets
// Pattern on its own copy of the request.
func TestRequestLogCarriesRouteUserAndClient(t *testing.T) {
	e := newEnv(t)
	tok := e.signIn(t, "sub-log", "logger")
	for _, ua := range []string{"repose-cli", "Mozilla/5.0 (X11)", ""} {
		req, _ := http.NewRequest("GET", e.api.URL+"/v1/me", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", ua)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
	}
	want := map[string]bool{"cli": false, "dashboard": false, "other": false}
	sc := bufio.NewScanner(bytes.NewReader(e.logs.Bytes()))
	for sc.Scan() {
		var l map[string]any
		if json.Unmarshal(sc.Bytes(), &l) != nil || l["event"] != "request" || l["route"] != "GET /v1/me" {
			continue
		}
		if l["user_id"] == nil || l["user_id"] == "" {
			t.Errorf("request line without user_id: %s", sc.Bytes())
		}
		if c, ok := l["client"].(string); ok {
			want[c] = true
		}
		if bytes.Contains(sc.Bytes(), []byte("Mozilla")) {
			t.Errorf("User-Agent leaked into the log: %s", sc.Bytes())
		}
	}
	for c, seen := range want {
		if !seen {
			t.Errorf("no GET /v1/me request line with client %q; log:\n%s", c, e.logs.String())
		}
	}
}
