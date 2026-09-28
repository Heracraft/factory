package cli

import (
	"testing"
)

// `repose browser [PROJECT]` and `repose browser bridge [PROJECT]` share a
// word (I-310): cobra picks the subcommand when the first word is
// "bridge", and the machine's own browser for anything else.
func TestBrowserCommandTreeParses(t *testing.T) {
	root := newRootCmd("dev")
	cases := []struct {
		args     []string
		cmd      string
		rest     []string
		badUsage bool
	}{
		{[]string{"browser"}, "repose browser", nil, false},
		{[]string{"browser", "todo-app"}, "repose browser", []string{"todo-app"}, false},
		{[]string{"browser", "--stop", "todo-app"}, "repose browser", []string{"todo-app"}, false},
		{[]string{"browser", "bridge"}, "repose browser bridge", nil, false},
		{[]string{"browser", "bridge", "todo-app"}, "repose browser bridge", []string{"todo-app"}, false},
		{[]string{"browser", "bridge", "--allow", "github.com", "todo-app"}, "repose browser bridge", []string{"todo-app"}, false},
		{[]string{"browser", "a", "b"}, "repose browser", []string{"a", "b"}, true},
	}
	for _, c := range cases {
		cmd, rest, err := root.Find(c.args)
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if cmd.CommandPath() != c.cmd {
			t.Errorf("%v: %s, want %s", c.args, cmd.CommandPath(), c.cmd)
			continue
		}
		if err := cmd.ParseFlags(rest); err != nil {
			t.Errorf("%v: flags: %v", c.args, err)
			continue
		}
		args := cmd.Flags().Args()
		err = cmd.ValidateArgs(args)
		if (err != nil) != c.badUsage {
			t.Errorf("%v: args %v: %v", c.args, args, err)
		}
		if !c.badUsage && len(args) != len(c.rest) {
			t.Errorf("%v: args %v, want %v", c.args, args, c.rest)
		}
	}
	// `repose open --desktop` stays: the same command under its old name.
	open, _, _ := root.Find([]string{"open"})
	if open.Flags().Lookup("desktop") == nil {
		t.Error("repose open --desktop is gone")
	}
}
