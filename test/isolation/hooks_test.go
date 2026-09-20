package isolation

import (
	"strings"
	"testing"
	"time"
)

// Row: the hook socket cannot be used to spoof another project. The payload
// has no project field at all; guestd stamps nothing but agent, kind,
// summary and window, and hostd stamps the guest id from its own table. A
// payload naming another project is accepted as a plain event for A, and a
// malformed one is rejected and logged `hook_bad_payload` with the reason
// only.
func TestForgedHookPayloadCannotNameAnotherProject(t *testing.T) {
	need(t, "EXEC_A", "HOST_EXEC", "A_GUEST_ID")
	marker := "repose-iso-" + time.Now().UTC().Format("150405")
	forged := `{"agent":"claude","kind":"completed","summary":"` + marker + `","project_id":"00000000-0000-7000-8000-000000000000","guest_id":"forged"}`
	r := inA(t, "curl -s -o /dev/null -w '%{http_code}' --unix-socket /run/repose/hooks.sock -H 'Content-Type: application/json' -d "+shellQuote(forged)+" http://repose/")
	if strings.TrimSpace(r.out) != "204" {
		t.Fatalf("hook socket answered %q to a payload with extra fields, want 204:\n%s", r.out, r)
	}
	bad := `{"agent":"evil","kind":"completed","summary":"x"}`
	r = inA(t, "curl -s -o /dev/null -w '%{http_code}' --unix-socket /run/repose/hooks.sock -H 'Content-Type: application/json' -d "+shellQuote(bad)+" http://repose/")
	if strings.TrimSpace(r.out) != "400" {
		t.Fatalf("hook socket answered %q to an unknown agent, want 400:\n%s", r.out, r)
	}
	g := inA(t, "sudo -n journalctl -u guestd --since '-2 min' --no-pager -o cat | grep -c hook_bad_payload")
	if strings.TrimSpace(g.out) == "0" || g.code != 0 && strings.TrimSpace(g.out) == "" {
		t.Fatalf("guestd did not log hook_bad_payload for the unknown agent:\n%s", g)
	}
	// hostd attributed the accepted event to A's guest id, never to the forged one.
	h := onHost(t, "journalctl -u hostd --since '-2 min' --no-pager -o cat | grep agent_event | tail -n 5")
	if !strings.Contains(h.out, env("A_GUEST_ID")) {
		t.Fatalf("hostd logged no agent_event for guest A:\n%s", h)
	}
	if strings.Contains(h.out, "forged") || strings.Contains(h.out, "00000000-0000-7000-8000-000000000000") {
		t.Fatalf("hostd carried the forged project or guest id:\n%s", h)
	}
	if strings.Contains(h.out, marker) {
		t.Fatalf("hostd logged the hook summary (agent output) in its journal:\n%s", h)
	}
}
