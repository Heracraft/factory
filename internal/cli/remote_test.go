package cli

import "testing"

func TestNormalizeRemote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"git@github.com:A/B.git", "github.com/a/b"},
		{"https://github.com/a/b", "github.com/a/b"},
		{"https://github.com/a/b.git", "github.com/a/b"},
		{"https://GitHub.com/a/b", "github.com/a/b"},
		{"https://github.com/a/b/", "github.com/a/b"},
		{"ssh://git@github.com/a/b", "github.com/a/b"},
		{"ssh://git@github.com/a/b.git", "github.com/a/b"},
	}
	for _, c := range cases {
		if got := normalizeRemote(c.in); got != c.want {
			t.Errorf("normalizeRemote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
