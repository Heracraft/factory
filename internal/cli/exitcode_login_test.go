package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A refresh Logto refuses is an expired login; one that never got an
// answer is a network problem. Neither is reported as "Not logged in".
func TestExitCodeForLoginFailures(t *testing.T) {
	cases := []struct {
		cause error
		want  string
	}{
		{errors.New("not logged in"), "Not logged in. Run `repose login`."},
		{fmt.Errorf("refreshing session: %w", errors.New("invalid_grant: grant request is invalid")), "Your login has expired."},
		{fmt.Errorf("refreshing session: %w", errors.New("dial tcp: i/o timeout")), "Could not refresh your login: dial tcp: i/o timeout."},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if code := exitCodeFor(&notLoggedInError{cause: c.cause}, &buf); code != ExitNotLoggedIn {
			t.Errorf("%v: exit %d, want %d", c.cause, code, ExitNotLoggedIn)
		}
		if !strings.Contains(buf.String(), c.want) {
			t.Errorf("%v: printed %q, want it to contain %q", c.cause, buf.String(), c.want)
		}
	}
}
