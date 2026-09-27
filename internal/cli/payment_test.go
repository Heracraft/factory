package cli

import (
	"strings"
	"testing"
)

// payment_required prints the api's sentence verbatim and exits 7; an
// older api's fragment (or a Stripe-era reason) gets the plan sentence
// (DECISIONS I-289).
func TestPaymentRequiredMessage(t *testing.T) {
	const fallback = "Choose a plan at https://repose.herakraft.co/billing first."
	for _, c := range []struct {
		name string
		err  *APIError
		want string
	}{
		{"plan_limit verbatim", &APIError{Code: "payment_required", Message: "Your Solo plan runs 8 GB at once and todo-app is using it. Stop it, or upgrade at https://repose.herakraft.co/billing.", Detail: map[string]any{"reason": "plan_limit"}},
			"Your Solo plan runs 8 GB at once and todo-app is using it. Stop it, or upgrade at https://repose.herakraft.co/billing."},
		{"egress_limit verbatim", &APIError{Code: "payment_required", Message: "Your machines are stopped until 1 November: this period's egress passed 1000 GB.", Detail: map[string]any{"reason": "egress_limit"}},
			"Your machines are stopped until 1 November: this period's egress passed 1000 GB."},
		{"subscription_required verbatim", &APIError{Code: "payment_required", Message: fallback, Detail: map[string]any{"reason": "subscription_required", "waitlist": nil}}, fallback},
		{"old api card_required", &APIError{Code: "payment_required", Message: "add a card before starting a guest", Detail: map[string]any{"reason": "card_required"}}, fallback},
		{"old api no detail", &APIError{Code: "payment_required", Message: "your trial credit is used up"}, fallback},
		{"new reason, empty message", &APIError{Code: "payment_required", Detail: map[string]any{"reason": "past_due"}}, fallback},
	} {
		if got := paymentRequiredMessage(c.err); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	// The generic handler prints it and exits 7.
	var stderr strings.Builder
	code := exitCodeFor(&APIError{Code: "payment_required", Message: "Your last payment failed. Update your card at https://repose.herakraft.co/billing to start machines again.", Detail: map[string]any{"reason": "past_due"}}, &stderr)
	if code != ExitPaymentRequired || !strings.Contains(stderr.String(), "Your last payment failed.") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
}

// `repose status` and `repose ls` show hours, not cents (I-289).
func TestStatusShowsHours(t *testing.T) {
	for secs, want := range map[int64]string{0: "0h", 59: "0h", 60: "0h01m", 8040: "2h14m", 3600: "1h", 41 * 3600: "41h", 41*3600 + 1800: "41h", 90000: "25h"} {
		if got := runHours(secs); got != want {
			t.Errorf("runHours(%d) = %q, want %q", secs, got, want)
		}
	}
	p := &Project{Slug: "todo-app", Class: "large", State: "running", RunningSecondsToday: 8040, RunningSecondsMonth: 41 * 3600}
	line := statusFirstLine(p)
	if !strings.Contains(line, "today 2h14m  month 41h") || strings.Contains(line, "$") {
		t.Fatalf("status line %q", line)
	}
	var b strings.Builder
	writeProjectsTable(&b, []Project{*p})
	if !strings.Contains(b.String(), "TODAY") || !strings.Contains(b.String(), "2h14m") || !strings.Contains(b.String(), "41h") || strings.Contains(b.String(), "$") {
		t.Fatalf("table:\n%s", b.String())
	}
}
