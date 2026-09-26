package waitlist

import "testing"

func TestFleetFits(t *testing.T) {
	const gb = int64(1) << 30
	f := Fleet{Usable: 40 * gb, Reserved: 24 * gb}
	if !f.Fits(80, 8*gb) {
		t.Fatal("24 + 8 of 40 is 80 percent and fits")
	}
	if f.Fits(80, 8*gb+1) {
		t.Fatal("past 80 percent fits")
	}
	f.Pending = 1 // one admission counted as a large
	if f.Fits(80, 4*gb) {
		t.Fatal("24 + 8 pending + 4 of 40 fits")
	}
	if (Fleet{}).Fits(80, 0) {
		t.Fatal("an empty fleet fits")
	}
}

func TestMessageCarriesNoTenantInput(t *testing.T) {
	if got := Message(4, "a@example.com"); got != "repose is at capacity. You're number 4 on the waitlist; we'll email a@example.com when there's room." {
		t.Fatalf("%q", got)
	}
	if got := Message(1, ""); got != "repose is at capacity. You're number 1 on the waitlist. Run `repose run` again later." {
		t.Fatalf("%q", got)
	}
}
