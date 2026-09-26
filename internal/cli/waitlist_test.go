package cli

import (
	"context"
	"strings"
	"testing"

	fakeapi "github.com/heracraft/repose/internal/fakes/api"
)

// DECISIONS I-269: a first project refused with `waitlisted` prints the
// place and the address the email goes to, and exits 8 like capacity.
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
	want := "repose is at capacity. You're number 3 on the waitlist; we'll email " + fakeapi.CannedUser.Email + " when there's room.\n"
	if out.String() != want {
		t.Fatalf("printed %q, want %q", out.String(), want)
	}
	fake.SetWaitlisted(0)
	if _, err := createProjectForRun(ctx, e, "", RunOptions{Name: "todo-app"}, nil); err != nil {
		t.Fatalf("after admission: %v", err)
	}
}

func TestWaitlistedMessageFallsBack(t *testing.T) {
	// No detail (a proxy, or an api that changes shape): the api's own
	// sentence is printed as it is.
	e := &APIError{Code: "waitlisted", Message: "repose is at capacity."}
	if got := waitlistedMessage(e); got != "repose is at capacity." {
		t.Fatalf("got %q", got)
	}
	e = &APIError{Code: "waitlisted", Detail: map[string]any{"position": float64(2)}}
	if got := waitlistedMessage(e); got != "repose is at capacity. You're number 2 on the waitlist. Run `repose run` again later." {
		t.Fatalf("got %q", got)
	}
}
