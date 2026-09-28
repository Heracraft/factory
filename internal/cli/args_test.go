package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// argErrorExempt names commands whose argument check refuses too many
// words with a message that is about something else, and why.
var argErrorExempt = map[string]string{
	// Without -- the missing -- is the mistake, and the error says so.
	"repose exec": "wants -- before the command",
}

// TestArgErrorsSayWhatTheyGot holds every command's argument check to
// I-346: a refusal is a usage error (exit 2) and names what it received.
// `repose cp ./Fwd_* izma:/tmp` once printed the same "takes a source and
// a destination" however the glob expanded, so the user never saw that
// the shell had passed four words.
func TestArgErrorsSayWhatTheyGot(t *testing.T) {
	many := []string{"w1", "w2", "w3", "w4", "w5", "w6", "w7", "w8", "w9"}
	walkCommands(docsRoot(), func(c *cobra.Command) {
		path := c.CommandPath()
		if c.Args == nil || !c.Runnable() || c.Hidden { // hidden ones are run by repose, not typed
			return
		}
		if _, ok := argErrorExempt[path]; ok {
			return
		}
		for _, args := range [][]string{many, nil} {
			err := c.Args(c, args)
			if err == nil {
				continue
			}
			if _, ok := err.(cobraUsageError); !ok && !isCobraRefusal(err) {
				t.Errorf("%s %v: %q is not a usage error; return cobraUsageError", path, args, err)
			}
			msg := err.Error()
			said := strings.Contains(msg, "9") || strings.Contains(msg, "w9")
			if len(args) == 0 {
				said = strings.Contains(msg, "no argument") || strings.Contains(msg, "received 0")
			}
			if !said {
				t.Errorf("%s with %d arguments: %q does not say what it got; use gotArgs", path, len(args), msg)
			}
		}
	})
}

func TestCpArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "got no arguments"},
		{[]string{"./Fwd_a"}, "got 1 argument: ./Fwd_a"},
	} {
		err := newCpCmd(nil, &globalFlags{}).Args(nil, c.args)
		if !isUsage(err) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("cp %v: %v, want %q", c.args, err, c.want)
		}
	}
	if err := newCpCmd(nil, &globalFlags{}).Args(nil, []string{"./a", "./b", "./c", "izma:/tmp/"}); err != nil {
		t.Errorf("three sources and a destination: %v", err)
	}
}
