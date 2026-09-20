package isolation

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func apiGet(t *testing.T, path, token string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, strings.TrimSuffix(env("API_URL"), "/")+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body
}

// Row: a user cannot read another user's project. 404, not 403: existence
// is not leaked.
func TestOtherUsersProjectIs404NotForbidden(t *testing.T) {
	need(t, "API_URL", "TOKEN_A", "PROJECT_A", "PROJECT_B")
	code, _ := apiGet(t, "/projects/"+env("PROJECT_A"), env("TOKEN_A"))
	if code != 200 {
		t.Fatalf("A's own project: status %d", code)
	}
	code, body := apiGet(t, "/projects/"+env("PROJECT_B"), env("TOKEN_A"))
	if code != 404 {
		t.Fatalf("B's project as A: status %d, want 404 (never 403):\n%s", code, body)
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.Error.Code != "not_found" {
		t.Fatalf("error code %q, want not_found:\n%s", e.Error.Code, body)
	}
	for _, sub := range []string{"/secrets", "/snapshots", "/events", "/route", "/config"} {
		if code, _ := apiGet(t, "/projects/"+env("PROJECT_B")+sub, env("TOKEN_A")); code != 404 {
			t.Fatalf("B's project%s as A: status %d, want 404", sub, code)
		}
	}
}

// Row: a user cannot read secret values; the api never returns them.
func TestSecretsListHasNoValues(t *testing.T) {
	need(t, "API_URL", "TOKEN_A", "PROJECT_A")
	code, body := apiGet(t, "/projects/"+env("PROJECT_A")+"/secrets", env("TOKEN_A"))
	if code != 200 {
		t.Fatalf("secrets list: status %d\n%s", code, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("secrets list is not a JSON array: %v\n%s", err, body)
	}
	for _, row := range rows {
		for k := range row {
			switch k {
			case "name", "created_at", "updated_at":
			default:
				t.Fatalf("secrets list carries field %q; only name and timestamps are allowed", k)
			}
		}
	}
	t.Logf("%d secrets listed, no value field", len(rows))
}
