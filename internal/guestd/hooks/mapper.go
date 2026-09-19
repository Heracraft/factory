package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrNoEvent means the payload is a hook this agent fires that repose does not
// report. repose-hook exits 0 on it: a hook that fails must never block an
// agent, and neither must a hook that has nothing to say.
var ErrNoEvent = errors.New("hooks: payload carries no reportable event")

// TranscriptTail is how much of a Claude Code transcript is read to find the
// last assistant line (docs/workstreams/04-guestd.md §5).
const TranscriptTail = 4 << 10

// SummaryChars is how much of that line is kept.
const SummaryChars = 200

// Map turns one agent's native hook payload into the {agent, kind, summary}
// that the hook socket accepts. agent must be one of the five; the payload
// shapes are recorded in testdata beside this file and in features/agents.md.
func Map(agent string, payload []byte) (Payload, error) {
	if !agents[agent] {
		return Payload{}, fmt.Errorf("hooks: %q is not a known agent", agent)
	}
	// A wrapper may already speak the socket's own shape.
	if p, ok := asNative(agent, payload); ok {
		return p, nil
	}
	switch agent {
	case "claude":
		return mapClaude(payload)
	case "codex":
		return mapCodex(agent, payload)
	case "opencode":
		return mapOpencode(agent, payload)
	case "gemini", "pi":
		// features/agents.md: these have no completion hook in the shipped
		// versions; guestd's pane-idle heuristic reports them instead.
		return Payload{}, ErrNoEvent
	}
	return Payload{}, ErrNoEvent
}

// asNative accepts the socket's own shape, so a wrapper that can emit it
// directly needs no mapping at all.
func asNative(agent string, payload []byte) (Payload, bool) {
	var p Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return Payload{}, false
	}
	if p.Kind == "" || !kinds[p.Kind] {
		return Payload{}, false
	}
	if p.Agent == "" {
		p.Agent = agent
	}
	p.Summary = truncate(p.Summary, SummaryCap)
	return p, true
}

// claudeHook is the part of Claude Code's hook payload repose reads.
type claudeHook struct {
	HookEventName    string `json:"hook_event_name"`
	TranscriptPath   string `json:"transcript_path"`
	Message          string `json:"message"`
	NotificationType string `json:"notification_type"`
	Error            string `json:"error"`
}

// needsInputNotifications are the notification types that mean the agent is
// waiting for the user (docs/workstreams/04-guestd.md §5).
var needsInputNotifications = map[string]bool{
	"permission_prompt": true,
	"idle_prompt":       true,
	"agent_needs_input": true,
}

func mapClaude(payload []byte) (Payload, error) {
	var h claudeHook
	if err := json.Unmarshal(payload, &h); err != nil {
		return Payload{}, fmt.Errorf("hooks: parse claude payload: %w", err)
	}
	switch h.HookEventName {
	case "Stop":
		return Payload{
			Agent:   "claude",
			Kind:    "completed",
			Summary: lastAssistantLine(h.TranscriptPath),
		}, nil
	case "Notification":
		// Older builds carry only `message`; classify from it when the
		// explicit type is absent, so the wrapper works on both.
		nt := h.NotificationType
		if nt == "" {
			nt = classifyClaudeMessage(h.Message)
		}
		if !needsInputNotifications[nt] {
			return Payload{}, ErrNoEvent
		}
		return Payload{Agent: "claude", Kind: "needs_input", Summary: truncate(h.Message, SummaryCap)}, nil
	case "StopFailure":
		summary := h.Error
		if summary == "" {
			summary = h.Message
		}
		return Payload{Agent: "claude", Kind: "error", Summary: truncate(summary, SummaryCap)}, nil
	default:
		return Payload{}, ErrNoEvent
	}
}

// classifyClaudeMessage recognises the two notification messages Claude Code
// sends without a type: a permission request and an idle wait.
func classifyClaudeMessage(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "permission"):
		return "permission_prompt"
	case strings.Contains(lower, "waiting for your input"), strings.Contains(lower, "is idle"):
		return "idle_prompt"
	default:
		return ""
	}
}

// codexHook is Codex CLI's notify argument.
type codexHook struct {
	Type                 string `json:"type"`
	LastAssistantMessage string `json:"last-assistant-message"`
}

func mapCodex(agent string, payload []byte) (Payload, error) {
	var h codexHook
	if err := json.Unmarshal(payload, &h); err != nil {
		return Payload{}, fmt.Errorf("hooks: parse codex payload: %w", err)
	}
	switch h.Type {
	case "agent-turn-complete":
		return Payload{Agent: agent, Kind: "completed", Summary: clip(h.LastAssistantMessage)}, nil
	default:
		return Payload{}, ErrNoEvent
	}
}

// opencodeHook is opencode's event payload.
type opencodeHook struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	Properties struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	} `json:"properties"`
}

func mapOpencode(agent string, payload []byte) (Payload, error) {
	var h opencodeHook
	if err := json.Unmarshal(payload, &h); err != nil {
		return Payload{}, fmt.Errorf("hooks: parse opencode payload: %w", err)
	}
	summary := h.Properties.Message
	if summary == "" {
		summary = h.Message
	}
	switch h.Type {
	case "session.idle":
		return Payload{Agent: agent, Kind: "completed", Summary: clip(summary)}, nil
	case "session.error":
		if summary == "" {
			summary = h.Properties.Error
		}
		return Payload{Agent: agent, Kind: "error", Summary: clip(summary)}, nil
	case "permission.asked", "permission.updated":
		return Payload{Agent: agent, Kind: "needs_input", Summary: clip(summary)}, nil
	default:
		return Payload{}, ErrNoEvent
	}
}

// transcriptLine is the part of a Claude Code transcript entry that holds the
// assistant's words.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// lastAssistantLine reads the tail of a transcript and returns the most recent
// assistant text, clipped. A missing or unreadable transcript is not an error:
// the event is still worth sending, just without a summary.
func lastAssistantLine(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck // read-only

	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	size := fi.Size()
	offset := int64(0)
	if size > TranscriptTail {
		offset = size - TranscriptTail
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(buf), "\n")
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:] // the first line is a fragment of an earlier record
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || !strings.Contains(line, `"type":"assistant"`) {
			continue
		}
		var tl transcriptLine
		if err := json.Unmarshal([]byte(line), &tl); err != nil || tl.Type != "assistant" {
			continue
		}
		var parts []string
		for _, c := range tl.Message.Content {
			if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
				parts = append(parts, strings.TrimSpace(c.Text))
			}
		}
		if len(parts) == 0 {
			continue
		}
		return clip(strings.Join(parts, " "))
	}
	return ""
}

// clip collapses whitespace and keeps the first SummaryChars characters, so a
// summary is one line the user can read in a notification.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) <= SummaryChars {
		return s
	}
	return string([]rune(s)[:SummaryChars]) + "…"
}
