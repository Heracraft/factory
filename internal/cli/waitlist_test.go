package cli

import (
	"context"
	"strings"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// DECISIONS I-269, I-290: a create refused with `waitlisted` prints the
// api's sentence as it is (the place and the address the email goes to)
// and exits 8 like capacity.
func TestRunWaitlistedPrintsPlaceAndExits8(t *testing.T) {
	fake := fakeapi.New(fakeapi.Options{})
	defer fake.Close()
	fake.SetWaitlisted(3)
	e := newLifecycleEnv(t, fake)
	ctx := context.Background()
	_, err := createProjectForRun(ctx, e, "", RunOptions{Name: "todo-app"}, nil)
	if err == nil {
		t.Fatal("create went through while waitlisted")
	}
	var out strings.Builder
	if code := exitCodeFor(err, &out); code != ExitCapacity {
		t.Fatalf("exit %d, want %d (%s)", code, ExitCapacity, out.String())
	}
	want := "repose is full right now. You're number 3 on the waitlist; we'll email " + fakeapi.CannedUser.Email + " when there's a seat.\n"
	if out.String() != want {
		t.Fatalf("printed %q, want %q", out.String(), want)
	}
	fake.SetWaitlisted(0)
	if _, err := createProjectForRun(ctx, e, "", RunOptions{Name: "todo-app"}, nil); err != nil {
		t.Fatalf("after admission: %v", err)
	}
}

func TestWaitlistedMessageFallsBack(t *testing.T) {
	// The api's own sentence wins, whatever the detail says.
	e := &APIError{Code: "waitlisted", Message: "repose is full right now.", Detail: map[string]any{"position": float64(9)}}
	if got := waitlistedMessage(e); got != "repose is full right now." {
		t.Fatalf("got %q", got)
	}
	// No message (a proxy that stripped it): built from the detail.
	e = &APIError{Code: "waitlisted", Detail: map[string]any{"position": float64(2)}}
	if got := waitlistedMessage(e); got != "repose is full right now. You're number 2 on the waitlist. The dashboard's plan page shows your place." {
		t.Fatalf("got %q", got)
	}
	e = &APIError{Code: "waitlisted"}
	if got := waitlistedMessage(e); got != "repose is full right now. You're on the waitlist; we'll email you when there's a seat." {
		t.Fatalf("got %q", got)
	}
}
