package project

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func quietLog() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func newHandler(t *testing.T) (*Handler, sysdep.Paths, *sysdep.FakeRunner) {
	t.Helper()
	p := sysdep.Paths{Root: t.TempDir()}
	run := sysdep.NewFakeRunner()
	// systemctl is-active returns non-zero when the unit is not running, which
	// is the state a fresh guest is in; the start that follows succeeds.
	run.Match["is-active"] = sysdep.RunResult{ExitCode: 3}
	h := New(p, run, quietLog())
	h.uid, h.gid = -1, -1
	return h, p, run
}

func req() *guestdv1.SetupProject {
	return &guestdv1.SetupProject{
		ProjectSlug: "todo-app",
		RemoteUrl:   "https://github.com/heracraft/todo-app",
		Tz:          "Africa/Nairobi",
		Lang:        "C.UTF-8",
	}
}

func TestSetupWritesEverything(t *testing.T) {
	h, p, run := newHandler(t)
	if err := h.Setup(context.Background(), req()); err != nil {
		t.Fatalf("setup: %v", err)
	}

	b, err := os.ReadFile(p.ProjectJSON())
	if err != nil {
		t.Fatalf("project.json: %v", err)
	}
	var info Info
	if err := json.Unmarshal(b, &info); err != nil {
		t.Fatalf("project.json is not JSON: %v", err)
	}
	if info.Slug != "todo-app" || info.TZ != "Africa/Nairobi" {
		t.Fatalf("project.json = %+v", info)
	}

	env, err := os.ReadFile(p.EtcEnv())
	if err != nil {
		t.Fatalf("/etc/repose/env: %v", err)
	}
	for _, want := range []string{"REPOSE_PROJECT=todo-app", "TZ=Africa/Nairobi", "LANG=C.UTF-8"} {
		if !strings.Contains(string(env), want) {
			t.Errorf("/etc/repose/env is missing %q:\n%s", want, env)
		}
	}

	if fi, err := os.Stat(p.ProjectDir("todo-app")); err != nil || !fi.IsDir() {
		t.Fatalf("project directory: %v", err)
	}
	if _, ok := run.Ran("git init"); !ok {
		t.Fatalf("git init was not run; calls: %v", run.Calls())
	}
	if _, ok := run.Ran("start " + TmuxUnit); !ok {
		t.Fatalf("%s was not started; calls: %v", TmuxUnit, run.Calls())
	}
	if h.Slug() != "todo-app" {
		t.Fatalf("Slug() = %q", h.Slug())
	}
}

func TestSetupUsesTheProjectJSONHostdSent(t *testing.T) {
	h, p, _ := newHandler(t)
	r := req()
	r.ProjectJson = []byte(`{"project_id":"01931f0e-0000-7000-8000-000000000001","slug":"todo-app","class":"large"}`)

	if err := h.Setup(context.Background(), r); err != nil {
		t.Fatalf("setup: %v", err)
	}
	b, _ := os.ReadFile(p.ProjectJSON())
	var info Info
	if err := json.Unmarshal(b, &info); err != nil {
		t.Fatal(err)
	}
	if info.ProjectID == "" || info.Class != "large" {
		t.Fatalf("the api's record was not written through: %+v", info)
	}
}

func TestSetupRejectsMalformedProjectJSON(t *testing.T) {
	h, _, _ := newHandler(t)
	r := req()
	r.ProjectJson = []byte("{not json")
	if err := h.Setup(context.Background(), r); sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
		t.Fatalf("code = %s, want invalid_argument", sysdep.CodeOf(err))
	}
}

func TestSetupIsIdempotent(t *testing.T) {
	h, p, run := newHandler(t)
	ctx := context.Background()
	if err := h.Setup(ctx, req()); err != nil {
		t.Fatalf("first setup: %v", err)
	}
	// Pretend the repository now exists and the tmux unit is up.
	if err := os.MkdirAll(filepath.Join(p.ProjectDir("todo-app"), ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	run.Match["is-active"] = sysdep.RunResult{ExitCode: 0}
	run.Reset()

	if err := h.Setup(ctx, req()); err != nil {
		t.Fatalf("second setup: %v", err)
	}
	if _, ok := run.Ran("git init"); ok {
		t.Fatal("git init ran again over an existing repository")
	}
	if _, ok := run.Ran("start " + TmuxUnit); ok {
		t.Fatal("the tmux unit was started again while it was already active")
	}
}

func TestSetupRejectsSlugsThatCouldEscape(t *testing.T) {
	h, _, _ := newHandler(t)
	for _, slug := range []string{"", "..", "../etc", "has space", "UPPER", "/abs", strings.Repeat("x", 65)} {
		r := req()
		r.ProjectSlug = slug
		if err := h.Setup(context.Background(), r); err == nil {
			t.Errorf("%q was accepted as a slug", slug)
		} else if sysdep.CodeOf(err) != sysdep.CodeInvalidArgument {
			t.Errorf("%q: code = %s, want invalid_argument", slug, sysdep.CodeOf(err))
		}
	}
}

func TestSlugIsLoadedFromDiskAtStart(t *testing.T) {
	h, p, _ := newHandler(t)
	if err := h.Setup(context.Background(), req()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// A guestd restart: a new handler over the same root must know the
	// session name before hostd sends SetupProject again.
	fresh := New(p, sysdep.NewFakeRunner(), quietLog())
	if fresh.Slug() != "todo-app" {
		t.Fatalf("Slug() after restart = %q, want todo-app", fresh.Slug())
	}
}

func TestSetupReportsAFailedTmuxStart(t *testing.T) {
	h, _, run := newHandler(t)
	run.Match["start "+TmuxUnit] = sysdep.RunResult{ExitCode: 1, Stderr: []byte("Failed to start")}
	if err := h.Setup(context.Background(), req()); err == nil {
		t.Fatal("a failed tmux unit start was reported as success")
	}
}
