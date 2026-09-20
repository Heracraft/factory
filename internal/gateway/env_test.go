package gateway

import (
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestEnvFilter(t *testing.T) {
	cases := map[string]bool{
		"TERM": true, "LANG": true, "COLORTERM": true, "TZ": true, "LC_ALL": true, "LC_CTYPE": true,
		"PATH": false, "LD_PRELOAD": false, "HOME": false, "SSH_AUTH_SOCK": false, "": false, "lc_all": false,
	}
	for name, want := range cases {
		payload := ssh.Marshal(envRequest{Name: name, Value: "x"})
		if _, got := filterEnv(payload); got != want {
			t.Errorf("%q: got %v want %v", name, got, want)
		}
	}
	if _, ok := filterEnv([]byte{0xff}); ok {
		t.Error("garbage payload passed the filter")
	}
}
