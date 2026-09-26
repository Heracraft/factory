package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

func TestParseResizeArgs(t *testing.T) {
	for _, c := range []struct {
		args    []string
		flag    string
		project string
		bytes   int64
		usage   bool
	}{
		{args: nil, project: ""},
		{args: []string{"80G"}, project: "", bytes: 80 << 30},
		{args: []string{"80g"}, flag: "izma", project: "izma", bytes: 80 << 30},
		{args: []string{"izma"}, project: "izma"},
		{args: []string{"izma", "120G"}, project: "izma", bytes: 120 << 30},
		{args: []string{"izma", "120G"}, flag: "izma", project: "izma", bytes: 120 << 30},
		{args: []string{"izma"}, flag: "other", usage: true},
		{args: []string{"izma", "big"}, usage: true},
	} {
		p, b, err := parseResizeArgs(c.args, &globalFlags{project: c.flag})
		if c.usage {
			if !isUsage(err) {
				t.Errorf("%v --project %q: want a usage error, got %v", c.args, c.flag, err)
			}
			continue
		}
		if err != nil || p != c.project || b != c.bytes {
			t.Errorf("%v --project %q: got %q %d %v, want %q %d", c.args, c.flag, p, b, err, c.project, c.bytes)
		}
	}
}

// TestResizeTakesProject is the annoyance found live on 2026-09-26:
// `repose resize --size large e2e-fr` failed with `"E2E-FR" is not a size
// like 80G`. Through the real command tree, the project is now the first
// argument like everywhere else, and a lone size is still the disk.
func TestResizeTakesProject(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("REPOSE_API_URL", fake.URL()+"/v1")
	t.Setenv("REPOSE_PROJECT", "")
	if err := os.MkdirAll(filepath.Join(cfg, "repose"), 0o700); err != nil {
		t.Fatal(err)
	}
	creds, _ := json.Marshal(Credentials{AccessToken: "tok", ExpiresAt: time.Now().Add(time.Hour), LogtoIssuer: "https://auth.example"})
	if err := os.WriteFile(filepath.Join(cfg, "repose", "credentials.json"), creds, 0o600); err != nil {
		t.Fatal(err)
	}
	client := newClient(fake.URL()+"/v1", staticToken("tok"))
	ctx := context.Background()
	izma, err := client.CreateProject(ctx, CreateProjectRequest{Name: "izma", Class: "large"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.StopProject(ctx, izma.ID, false); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) error {
		root := newRootCmd("test")
		root.SetArgs(args)
		root.SetOut(&strings.Builder{})
		root.SetErr(&strings.Builder{})
		return root.ExecuteContext(ctx)
	}
	quietStdout(t)

	if err := run("resize", "--size", "small", "izma"); err != nil {
		t.Fatalf("repose resize --size small izma: %v", err)
	}
	if p, _ := client.GetProject(ctx, izma.ID); p.Class != "small" {
		t.Fatalf("izma is %s after resize --size small izma", p.Class)
	}
	if err := run("resize", "izma", "--size", "xl", "--yes"); err != nil {
		t.Fatalf("repose resize izma --size xl: %v", err)
	}
	if p, _ := client.GetProject(ctx, izma.ID); p.Class != "xl" {
		t.Fatalf("izma is %s after resize izma --size xl", p.Class)
	}
	if err := run("resize", "izma", "80G"); err != nil {
		t.Fatalf("repose resize izma 80G: %v", err)
	}
	if err := run("resize", "100G", "--project", "izma"); err != nil {
		t.Fatalf("repose resize 100G --project izma: %v", err)
	}
	if err := run("resize", "izma"); !isUsage(err) {
		t.Fatalf("resize with a project and nothing to change: %v", err)
	}
	if err := run("resize", "izma", "huge"); !isUsage(err) {
		t.Fatalf("resize izma huge: %v", err)
	}
	if err := run("resize", "izma", "80G", "extra"); !isUsage(err) {
		t.Fatalf("three arguments: %v", err)
	}
	if err := run("resize", "izma", "--project", "other", "--size", "small"); !isUsage(err) {
		t.Fatalf("conflicting positional and --project: %v", err)
	}
}
