package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture reads a recorded hook payload. Every mapping this package claims is
// backed by one of these, so a change in an agent's payload shape shows up as
// a failing test rather than as a notification that never arrives.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func TestMapClaudeStop(t *testing.T) {
	transcript := filepath.Join("testdata", "claude", "transcript.jsonl")
	payload := strings.ReplaceAll(string(fixture(t, "claude/stop.json")), "TRANSCRIPT", transcript)

	p, err := Map("claude", []byte(payload))
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Agent != "claude" || p.Kind != "completed" {
		t.Fatalf("payload = %+v, want a claude completion", p)
	}
	if p.Summary != "Added 14 tests for the payment module and they all pass." {
		t.Fatalf("summary = %q, want the last assistant line", p.Summary)
	}
}

func TestMapClaudeStopWithoutATranscript(t *testing.T) {
	p, err := Map("claude", fixture(t, "claude/stop-no-transcript.json"))
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Kind != "completed" {
		t.Fatalf("kind = %q, want completed even with no transcript", p.Kind)
	}
	if p.Summary != "" {
		t.Fatalf("summary = %q, want empty", p.Summary)
	}
}

func TestMapClaudeNotifications(t *testing.T) {
	cases := []struct {
		file    string
		summary string
	}{
		{"claude/notification-permission-prompt.json", "Claude needs your permission to use Bash"},
		{"claude/notification-idle-prompt.json", "Claude is waiting for your input"},
		{"claude/notification-agent-needs-input.json", "The agent is waiting for an answer"},
		{"claude/notification-untyped.json", "Claude needs your permission to use Write"},
	}
	for _, c := range cases {
		p, err := Map("claude", fixture(t, c.file))
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if p.Kind != "needs_input" {
			t.Errorf("%s: kind = %q, want needs_input", c.file, p.Kind)
		}
		if p.Summary != c.summary {
			t.Errorf("%s: summary = %q, want %q", c.file, p.Summary, c.summary)
		}
	}
}

func TestMapClaudeIgnoresUnreportableHooks(t *testing.T) {
	for _, file := range []string{"claude/subagent-stop.json", "claude/notification-unrelated.json"} {
		if _, err := Map("claude", fixture(t, file)); !errors.Is(err, ErrNoEvent) {
			t.Errorf("%s: err = %v, want ErrNoEvent", file, err)
		}
	}
}

func TestMapClaudeStopFailure(t *testing.T) {
	p, err := Map("claude", fixture(t, "claude/stopfailure.json"))
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Kind != "error" {
		t.Fatalf("kind = %q, want error", p.Kind)
	}
	if !strings.Contains(p.Summary, "API error") {
		t.Fatalf("summary = %q", p.Summary)
	}
}

func TestMapCodex(t *testing.T) {
	p, err := Map("codex", fixture(t, "codex/agent-turn-complete.json"))
	if err != nil {
		t.Fatalf("map: %v", err)
	}
	if p.Agent != "codex" || p.Kind != "completed" {
		t.Fatalf("payload = %+v", p)
	}
	if p.Summary != "The auth flow is finished and the tests pass." {
		t.Fatalf("summary = %q", p.Summary)
	}
	if _, err := Map("codex", fixture(t, "codex/other-event.json")); !errors.Is(err, ErrNoEvent) {
		t.Fatalf("err = %v, want ErrNoEvent", err)
	}
}

func TestMapOpencode(t *testing.T) {
	cases := map[string]string{
		"opencode/session-idle.json":     "completed",
		"opencode/session-error.json":    "error",
		"opencode/permission-asked.json": "needs_input",
	}
	for file, want := range cases {
		p, err := Map("opencode", fixture(t, file))
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		if p.Kind != want {
			t.Errorf("%s: kind = %q, want %q", file, p.Kind, want)
		}
		if p.Summary == "" {
			t.Errorf("%s: summary is empty", file)
		}
	}
	if _, err := Map("opencode", fixture(t, "opencode/message-part-updated.json")); !errors.Is(err, ErrNoEvent) {
		t.Fatalf("err = %v, want ErrNoEvent for a chatty event", err)
	}
}

func TestMapHooklessAgentsAcceptOnlyTheNativeShape(t *testing.T) {
	// features/agents.md: gemini and pi have no completion hook, so there is
	// nothing to map; the pane-idle heuristic reports them instead. A wrapper
	// that can speak the socket's own shape is still accepted.
	for _, agent := range []string{"gemini", "pi"} {
		if _, err := Map(agent, []byte(`{"type":"whatever"}`)); !errors.Is(err, ErrNoEvent) {
			t.Errorf("%s: err = %v, want ErrNoEvent", agent, err)
		}
		p, err := Map(agent, fixture(t, agent+"/heuristic.json"))
		if err != nil {
			t.Errorf("%s: %v", agent, err)
			continue
		}
		if p.Agent != agent || p.Kind != "completed" {
			t.Errorf("%s: payload = %+v", agent, p)
		}
	}
}

func TestMapRejectsUnknownAgents(t *testing.T) {
	if _, err := Map("aider", []byte(`{"kind":"completed"}`)); err == nil {
		t.Fatal("an agent the platform does not ship was accepted")
	}
}

func TestSummaryIsClippedToOneReadableLine(t *testing.T) {
	long := strings.Repeat("word ", 200)
	got := clip(long)
	if len([]rune(got)) > SummaryChars+1 {
		t.Fatalf("clip returned %d runes, want at most %d plus an ellipsis", len([]rune(got)), SummaryChars)
	}
	if strings.Contains(clip("a\nb\tc"), "\n") {
		t.Fatal("clip left a newline in a notification summary")
	}
}

func TestTranscriptTailIsReadFromTheEnd(t *testing.T) {
	// A transcript larger than the tail: the reader must still find the last
	// assistant line rather than give up.
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"padding padding padding"}]}}` + "\n")
	}
	b.WriteString(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"the final word"}]}}` + "\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := lastAssistantLine(path); got != "the final word" {
		t.Fatalf("lastAssistantLine = %q, want the final word", got)
	}
}
