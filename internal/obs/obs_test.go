package obs

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactsEveryForbiddenField(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger("api", &buf, slog.LevelDebug)
	planted := "PLANTED-VALUE-9f8e"
	keys := []string{"token", "access_token", "join_token", "secret", "secret_value", "password", "authorization",
		"cert", "host_cert", "key", "host_key", "private_key", "email", "user_email", "handle", "remote_url",
		"argv", "environ", "prompt", "summary", "fragment", "ntfy_url", "github_login", "public_key", "ciphertext"}
	for _, k := range keys {
		l.Info("x", "event", "test", k, planted)
	}
	out := buf.String()
	if strings.Contains(out, planted) {
		t.Fatalf("planted value leaked:\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not json: %s", line)
		}
		for _, f := range []string{"ts", "level", "component", "event", "msg"} {
			if _, ok := m[f]; !ok {
				t.Fatalf("line lacks %s: %s", f, line)
			}
		}
		if m["component"] != "api" {
			t.Fatalf("component: %s", line)
		}
	}
}

func TestAllowedKeysPass(t *testing.T) {
	var buf bytes.Buffer
	l := NewLogger("api", &buf, slog.LevelInfo)
	l.Info("x", "event", "cert_issue", "kind", "user", "cert_serial", 42, "project_id", "p1", "key_version", "v2")
	out := buf.String()
	for _, want := range []string{`"kind":"user"`, `"cert_serial":42`, `"project_id":"p1"`, `"key_version":"v2"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %s in %s", want, out)
		}
	}
}
