package isolation

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Row: console logs do not carry terminal contents. Text typed into the
// tmux session must never appear in the guest's console.log on the host,
// nor in Loki.
func TestConsoleLogCarriesNoTerminalContents(t *testing.T) {
	need(t, "EXEC_A", "HOST_EXEC", "A_GUEST_ID", "A_SLUG")
	marker := "REPOSE_ISO_TYPED_" + time.Now().UTC().Format("20060102150405")
	mustSucceed(t, inA(t, "tmux send-keys -t "+shellQuote(env("A_SLUG"))+" 'echo "+marker+"' Enter"), "type into A's tmux session")
	time.Sleep(3 * time.Second)
	mustSucceed(t, inA(t, "tmux capture-pane -p -t "+shellQuote(env("A_SLUG"))+" | grep -q "+marker), "the marker is on A's terminal")
	r := onHost(t, "grep -c "+marker+" /var/lib/repose/guests/"+env("A_GUEST_ID")+"/console.log* 2>/dev/null; true")
	for _, line := range strings.Split(strings.TrimSpace(r.out), "\n") {
		_, n, _ := strings.Cut(line, ":")
		if n == "" {
			n = line
		}
		if strings.TrimSpace(n) != "0" && strings.TrimSpace(n) != "" {
			t.Fatalf("typed text reached the console log:\n%s", r)
		}
	}
	if loki := env("LOKI_URL"); loki != "" {
		if _, err := exec.LookPath("logcli"); err == nil {
			out, _ := exec.Command("logcli", "query", "--addr", loki, "--since", "10m", "--quiet", `{component="console"} |= "`+marker+`"`).CombinedOutput()
			if strings.Contains(string(out), marker) {
				t.Fatalf("typed text reached Loki:\n%s", out)
			}
		} else {
			t.Log("logcli not installed; Loki half skipped")
		}
	}
	t.Log("typed marker absent from console.log")
}
