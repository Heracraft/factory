package guestd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	"github.com/heracraft/repose/internal/guestd/hooks"
	"github.com/heracraft/repose/internal/guestd/questions"
)

func hookClient(socket string) *http.Client {
	return &http.Client{Timeout: 40 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
	}}
}

func hookDo(t *testing.T, c *http.Client, method, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "http://guestd"+path, r)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// TestNotifyRelaysAsAnAgentMessage: POST /notify becomes an AgentEvent of
// kind agent_message; an unknown agent is attributed to "shell".
func TestNotifyRelaysAsAnAgentMessage(t *testing.T) {
	h := newHarness(t)
	c := hookClient(h.srv.HookSocketPath())
	code, _ := hookDo(t, c, http.MethodPost, "/notify", hooks.MessagePayload{Agent: "not-an-agent", Text: "  deploy is green  "})
	if code != http.StatusNoContent {
		t.Fatalf("POST /notify = %d", code)
	}
	n := h.waitForNotify(t, "agent_message", func(n *guestdv1.Notify) bool { return n.GetAgentEvent().GetKind() == hooks.KindMessage })
	ev := n.GetAgentEvent()
	if ev.GetAgent() != "shell" || ev.GetSummary() != "deploy is green" || ev.GetTmuxWindow() != "shell" {
		t.Fatalf("event = %+v", ev)
	}
	if code, _ := hookDo(t, c, http.MethodPost, "/notify", hooks.MessagePayload{Agent: "claude", Text: "   "}); code != http.StatusBadRequest {
		t.Fatalf("an empty message = %d, want 400", code)
	}
}

// TestAskIsAnsweredOverTheWire drives the whole guest side: the ask opens
// and is announced, a new hostd connection re-announces it, AnswerQuestion
// closes it, the long poll returns the answer, and a repeat answer is a
// no-op while an unknown id is not_found.
func TestAskIsAnsweredOverTheWire(t *testing.T) {
	h := newHarness(t)
	c := hookClient(h.srv.HookSocketPath())
	code, body := hookDo(t, c, http.MethodPost, "/ask", hooks.AskPayload{Agent: "codex", Window: "codex", Text: "Drop the legacy table?", Options: []string{"yes", "no", "YES"}, TimeoutS: 600})
	if code != http.StatusCreated {
		t.Fatalf("POST /ask = %d %s", code, body)
	}
	var st hooks.AskState
	if err := json.Unmarshal(body, &st); err != nil || st.ID == "" || st.State != questions.StateOpen {
		t.Fatalf("ask state = %s (%v)", body, err)
	}
	n := h.waitForNotify(t, "Question", func(n *guestdv1.Notify) bool { return n.GetQuestion().GetQuestionId() == st.ID })
	q := n.GetQuestion()
	if q.GetText() != "Drop the legacy table?" || strings.Join(q.GetOptions(), ",") != "yes,no" || q.GetTimeoutS() != 600 || q.GetState() != "" || q.GetAgent() != "codex" {
		t.Fatalf("question = %+v", q)
	}
	if _, err := os.Stat(filepath.Join(h.paths.QuestionsDir(), st.ID+".json")); err != nil {
		t.Fatalf("the question is not on the tmpfs: %v", err)
	}

	// A hostd restart: the new connection hears about the open question again.
	h.mu.Lock()
	h.notifies = nil
	h.mu.Unlock()
	h.client = h.dial(t)
	h.waitForNotify(t, "re-announced Question", func(n *guestdv1.Notify) bool { return n.GetQuestion().GetQuestionId() == st.ID })

	// A short poll while open answers "open".
	code, body = hookDo(t, c, http.MethodGet, "/ask/"+st.ID+"?wait=0", nil)
	if code != http.StatusOK || !strings.Contains(string(body), `"state":"open"`) {
		t.Fatalf("poll while open = %d %s", code, body)
	}

	got := make(chan hooks.AskState, 1)
	go func() {
		_, b := hookDo(t, c, http.MethodGet, "/ask/"+st.ID+"?wait=20", nil)
		var s hooks.AskState
		_ = json.Unmarshal(b, &s)
		got <- s
	}()
	time.Sleep(100 * time.Millisecond)
	answer := &guestdv1.Request{Req: &guestdv1.Request_AnswerQuestion{AnswerQuestion: &guestdv1.AnswerQuestion{QuestionId: st.ID, Status: "answered", Answer: "yes"}}}
	if resp := h.do(t, answer); !resp.GetOk() {
		t.Fatalf("AnswerQuestion: %+v", resp.GetError())
	}
	select {
	case s := <-got:
		if s.State != questions.StateAnswered || s.Answer != "yes" {
			t.Fatalf("the waiting poll got %+v", s)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the long poll did not return on the answer")
	}
	if resp := h.do(t, answer); !resp.GetOk() {
		t.Fatalf("a repeated AnswerQuestion is not a no-op: %+v", resp.GetError())
	}
	unknown := &guestdv1.Request{Req: &guestdv1.Request_AnswerQuestion{AnswerQuestion: &guestdv1.AnswerQuestion{QuestionId: "0199aaaa-0000-7000-8000-000000000000", Status: "answered"}}}
	if resp := h.do(t, unknown); resp.GetOk() || resp.GetError().GetCode() != "not_found" {
		t.Fatalf("unknown question = %+v, want not_found", resp)
	}
	if code, _ := hookDo(t, c, http.MethodGet, "/ask/nope?wait=0", nil); code != http.StatusNotFound {
		t.Fatalf("polling an unknown question = %d, want 404", code)
	}
}

// TestAskCancelledByTheAskerIsAnnounced: DELETE /ask/{id} closes it and
// hostd hears state cancelled.
func TestAskCancelledByTheAskerIsAnnounced(t *testing.T) {
	h := newHarness(t)
	c := hookClient(h.srv.HookSocketPath())
	_, body := hookDo(t, c, http.MethodPost, "/ask", hooks.AskPayload{Text: "ok to push?"})
	var st hooks.AskState
	_ = json.Unmarshal(body, &st)
	if code, _ := hookDo(t, c, http.MethodDelete, "/ask/"+st.ID, nil); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", code)
	}
	h.waitForNotify(t, "cancelled Question", func(n *guestdv1.Notify) bool {
		return n.GetQuestion().GetQuestionId() == st.ID && n.GetQuestion().GetState() == "cancelled"
	})
}

// TestQuestionsSurviveAGuestdRestart: a new store over the same directory
// knows the open question, which is what keeps an ask alive across a
// guestd restart.
func TestQuestionsSurviveAGuestdRestart(t *testing.T) {
	dir := t.TempDir()
	var emitted []questions.Question
	s1 := questions.New(dir, func(q questions.Question) { emitted = append(emitted, q) }, testLog(t), nil)
	q, err := s1.Open("claude", "claude", "merge?", nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	s2 := questions.New(dir, func(questions.Question) {}, testLog(t), nil)
	if p := s2.Pending(); len(p) != 1 || p[0].ID != q.ID {
		t.Fatalf("pending after restart = %+v", p)
	}
	if err := s2.Answer(q.ID, "answered", "go"); err != nil {
		t.Fatal(err)
	}
	got, _ := s2.Wait(context.Background(), q.ID)
	if got.Answer != "go" {
		t.Fatalf("answer = %q", got.Answer)
	}
}
