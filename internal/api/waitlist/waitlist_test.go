package waitlist

import "testing"

// The `waitlisted` sentence names the place and the address and nothing a
// tenant typed; the CLI prints it as it is (DECISIONS I-290).
func TestMessageCarriesNoTenantInput(t *testing.T) {
	if got := Message(4, "a@example.com"); got != "repose is full right now. You're number 4 on the waitlist; we'll email a@example.com when there's a seat." {
		t.Fatalf("%q", got)
	}
	if got := Message(1, ""); got != "repose is full right now. You're number 1 on the waitlist. The dashboard's plan page shows your place." {
		t.Fatalf("%q", got)
	}
}
