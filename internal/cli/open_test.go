package cli

import "testing"

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
