package cli

import (
	"context"
	"strings"
	"testing"
)

func TestDesktopPassword(t *testing.T) {
	cases := map[string]string{
		"s3cr3tpw\n":                    "s3cr3tpw",
		"Job started\n  s3cr3tpw  \n\n": "s3cr3tpw",
		"":                              "",
	}
	for out, want := range cases {
		if got := desktopPassword([]byte(out)); got != want {
			t.Errorf("desktopPassword(%q) = %q, want %q", out, got, want)
		}
	}
}

// --stop only means something with --desktop.
func TestOpenStopNeedsDesktop(t *testing.T) {
	root := newRootCmd("test")
	root.SetArgs([]string{"open", "--stop", "3000"})
	root.SetOut(&strings.Builder{})
	root.SetErr(&strings.Builder{})
	err := root.ExecuteContext(context.Background())
	if _, ok := err.(cobraUsageError); !ok {
		t.Fatalf("err = %T %v, want a usage error (exit 2)", err, err)
	}
}
