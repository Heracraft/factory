package guest

import (
	"testing"
	"time"

	guestdv1 "github.com/heracraft/repose/internal/gen/guestd/v1"
	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// TestQuestionNotifyBecomesAnAgentQuestion: a guest's repose-ask reaches
// the api as an AgentQuestion carrying every field (DECISIONS I-244).
func TestQuestionNotifyBecomesAnAgentQuestion(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	q := &guestdv1.Question{QuestionId: "0199aaaa-0000-7000-8000-00000000000a", Agent: "claude", TmuxWindow: "claude",
		Text: "Drop the legacy table?", Options: []string{"yes", "no"}, TimeoutS: 1800}
	if err := h.guestd(gid1).Notify(&guestdv1.Notify{N: &guestdv1.Notify_Question{Question: q}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.rec.mu.Lock()
		for _, e := range h.rec.events {
			if a := e.GetAgentQuestion(); a != nil {
				h.rec.mu.Unlock()
				if a.GuestId != gid1 || a.QuestionId != q.QuestionId || a.Text != q.Text || len(a.Options) != 2 || a.TimeoutS != 1800 || a.TmuxWindow != "claude" || a.State != "" {
					t.Fatalf("agent question lost fields: %+v", a)
				}
				return
			}
		}
		h.rec.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the question was not forwarded")
}

// TestAnswerQuestionReachesGuestd: the command is relayed as the guestd
// request of the same name, and a stopped guest is not_found, which the
// api treats as final.
func TestAnswerQuestionReachesGuestd(t *testing.T) {
	h := newHarness(t, nil)
	h.create(gid1)
	a := &hostdv1.AnswerQuestion{GuestId: gid1, QuestionId: "0199aaaa-0000-7000-8000-00000000000b", Status: "answered", Answer: "yes"}
	h.mustOK(cmd(a))
	var got *guestdv1.AnswerQuestion
	for _, c := range h.guestd(gid1).Calls() {
		if r := c.Req.GetAnswerQuestion(); r != nil {
			got = r
		}
	}
	if got == nil || got.QuestionId != a.QuestionId || got.Status != "answered" || got.Answer != "yes" {
		t.Fatalf("guestd got %+v", got)
	}

	h.mustFail(cmd(&hostdv1.AnswerQuestion{GuestId: gid1, QuestionId: a.QuestionId}), CodeInvalidArgument)

	h.mustOK(cmd(&hostdv1.StopGuest{GuestId: gid1}))
	h.mustFail(cmd(a), CodeNotFound)
}

// TestAnswerQuestionUnknownToGuestdIsNotFound: guestd's not_found (the
// guest rebooted and its tmpfs forgot the question) passes through as
// not_found rather than becoming internal.
func TestAnswerQuestionUnknownToGuestdIsNotFound(t *testing.T) {
	h := newHarness(t, nil)
	h.gopts.Fail = map[string]*guestdv1.Error{"AnswerQuestion": {Code: "not_found", Message: "no question"}}
	h.create(gid1)
	h.mustFail(cmd(&hostdv1.AnswerQuestion{GuestId: gid1, QuestionId: "q", Status: "answered"}), CodeNotFound)
}
