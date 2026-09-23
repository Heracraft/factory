package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	onMain  = "69b3229e0c3a3bd7cfa0e0e0b7b1c3d7d1f0a001"
	offMain = "1111111111111111111111111111111111111111"
	missing = "3f83664f00000000000000000000000000000000"
)

// TestBaseRevGate: base publish takes only a full sha that GitHub says is
// on the branch (I-173); short, unknown, off-branch and unanswerable revs
// are refused with a sentence that says what to do.
func TestBaseRevGate(t *testing.T) {
	var paths []string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("User-Agent") == "" {
			http.Error(w, "no UA", http.StatusForbidden)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "..."+onMain):
			_, _ = w.Write([]byte(`{"status":"behind","ahead_by":0,"behind_by":3}`))
		case strings.HasSuffix(r.URL.Path, "..."+offMain):
			_, _ = w.Write([]byte(`{"status":"diverged","ahead_by":2,"behind_by":3}`))
		case strings.HasSuffix(r.URL.Path, "..."+missing):
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		default:
			http.Error(w, "boom", http.StatusBadGateway)
		}
	}))
	defer gh.Close()
	old := githubAPI
	githubAPI = gh.URL
	defer func() { githubAPI = old }()

	e := &Env{}
	ctx := context.Background()
	got, err := e.checkBaseRev(ctx, strings.ToUpper(onMain), DefaultBaseRepo, "main")
	if err != nil || got != onMain {
		t.Fatalf("on main: %q %v", got, err)
	}
	if len(paths) != 1 || paths[0] != "/repos/Heracraft/factory/compare/main..."+onMain {
		t.Fatalf("asked %v", paths)
	}
	for rev, want := range map[string]string{
		"3f83664f":              "not a full commit sha",
		"main":                  "not a full commit sha",
		offMain:                 "not on main",
		missing:                 "has no commit",
		strings.Repeat("2", 40): "could not check",
	} {
		_, err := e.checkBaseRev(ctx, rev, DefaultBaseRepo, "main")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", rev, err, want)
		}
		if want != "could not check" && !errors.Is(err, ErrUsage) {
			t.Errorf("%s: %v is not a usage error", rev, err)
		}
	}
	if err := githubRevCheck(ctx, "https://gitlab.com/a/b.git", "main", onMain); err == nil {
		t.Fatal("a non-GitHub repository was 'checked'")
	}
	for _, repo := range []string{"git@github.com:Heracraft/factory.git", "https://github.com/Heracraft/factory", "ssh://git@github.com/Heracraft/factory.git"} {
		if err := githubRevCheck(ctx, repo, "main", onMain); err != nil {
			t.Errorf("%s: %v", repo, err)
		}
	}
}
