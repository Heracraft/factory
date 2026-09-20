package isolation

import (
	"strings"
	"testing"
)

// Row: guest A cannot reach guest B. Mechanism: per-guest tap attached
// isolated, `bridge repose` forward policy drop, static FDB, no shared L2.
func TestGuestACannotReachGuestB(t *testing.T) {
	need(t, "EXEC_A", "B_IP")
	b := env("B_IP")
	mustFail(t, inA(t, "ping -c 2 -W 2 "+b), "ping B from A")
	if r := inA(t, "command -v arping"); r.code == 0 {
		mustFail(t, inA(t, "sudo -n arping -c 2 -w 3 -I eth0 "+b), "arping B from A")
	} else {
		t.Log("arping not in the guest; ARP checked by the tcpdump half below")
	}
	for _, port := range []string{"22", "5000"} {
		mustFail(t, inA(t, "nc -z -w 3 "+b+" "+port), "connect A -> B:"+port)
	}
	if r := inA(t, "command -v nmap"); r.code == 0 {
		r := inA(t, "nmap -Pn -p 22,5000 --max-retries 1 --host-timeout 20s "+b)
		if strings.Contains(r.out, "open") {
			t.Fatalf("nmap from A sees an open port on B:\n%s", r)
		}
	}
	if env("HOST_EXEC") != "" && env("B_TAP") != "" && env("A_MAC") != "" {
		// Capture on B's tap while A talks; nothing from A's MAC may arrive.
		cap := "timeout 8 tcpdump -nn -e -i " + env("B_TAP") + " -c 1 'ether src " + env("A_MAC") + "' 2>/dev/null; echo captured=$?"
		done := make(chan result, 1)
		go func() { done <- onHost(t, cap) }()
		inA(t, "ping -c 3 -W 1 "+b+" >/dev/null 2>&1; nc -z -w 1 "+b+" 22 >/dev/null 2>&1; true")
		r := <-done
		if !strings.Contains(r.out, "captured=124") && !strings.Contains(r.out, "captured=1") {
			t.Fatalf("tcpdump on B's tap saw a frame from A's MAC:\n%s", r)
		}
		t.Log("tcpdump on B's tap saw nothing from A's MAC")
	} else {
		t.Log("HOST_EXEC, B_TAP or A_MAC unset; the tcpdump half of this row was not run")
	}
}

// Row: guest cannot reach the host. Mechanism: `guest_in` drops everything
// but rate-limited ICMP echo and replies to flows the host itself opened
// (host-conventions.md; DECISIONS I-18 kept a deliberate 5/s ping exception
// for debugging from a guest, and I-70 lets the host reach a guest's sshd;
// a connection a guest opens is the original direction and matches
// neither).
func TestGuestCannotReachHost(t *testing.T) {
	need(t, "EXEC_A", "HOST_IP")
	h := env("HOST_IP")
	for _, port := range []string{"22", "9100", "9101", "8080"} {
		mustFail(t, inA(t, "nc -z -w 3 "+h+" "+port), "connect A -> host:"+port)
	}
	mustFail(t, inA(t, "curl -sf -m 5 http://"+h+":9101/metrics"), "hostd metrics from A")
	// The echo exception is rate limited: a flood must lose most packets.
	r := inA(t, "ping -q -c 40 -i 0.01 -W 1 "+h+" 2>&1 | grep -o '[0-9]*% packet loss' || echo 'no ping output'")
	t.Logf("40-packet flood to the host: %s", strings.TrimSpace(r.out))
	if strings.Contains(r.out, " 0% packet loss") || strings.HasPrefix(strings.TrimSpace(r.out), "0% packet loss") {
		t.Fatal("ICMP echo to the host is not rate limited")
	}
}

// Row: guest cannot reach IMDS (and the Azure wire server).
func TestGuestCannotReachIMDS(t *testing.T) {
	need(t, "EXEC_A")
	mustFail(t, inA(t, "curl -s -m 5 -H Metadata:true 'http://169.254.169.254/metadata/instance?api-version=2021-02-01'"), "IMDS from A")
	mustFail(t, inA(t, "curl -s -m 5 'http://168.63.129.16/?comp=versions'"), "wire server from A")
}

// Row: guest cannot reach other hosts' guest ranges, or anything private.
func TestGuestCannotReachOtherHostsGuests(t *testing.T) {
	need(t, "EXEC_A", "OTHER_GUEST_IP")
	o := env("OTHER_GUEST_IP")
	mustFail(t, inA(t, "ping -c 2 -W 2 "+o), "ping another host's guest range from A")
	mustFail(t, inA(t, "nc -z -w 3 "+o+" 22"), "connect A -> other host's guest:22")
	for _, priv := range []string{"10.0.0.4", "172.16.0.1", "192.168.0.1", "100.64.0.1"} {
		mustFail(t, inA(t, "nc -z -w 2 "+priv+" 22"), "connect A -> private "+priv)
	}
	mustSucceed(t, inA(t, "curl -sfI -m 10 https://cache.nixos.org/nix-cache-info >/dev/null"), "egress to the internet from A")
}

// A guest cannot claim another guest's address: the bridge admits IPv4 and
// ARP only from the registered (mac, ip, tap) tuple.
func TestGuestCannotSpoofAnotherAddress(t *testing.T) {
	need(t, "EXEC_A", "B_IP", "HOST_IP")
	b := env("B_IP")
	defer inA(t, "sudo -n ip addr del "+b+"/22 dev eth0 2>/dev/null; true")
	mustSucceed(t, inA(t, "sudo -n ip addr add "+b+"/22 dev eth0"), "add B's address inside A (local operation)")
	mustFail(t, inA(t, "curl -sf -m 5 --interface "+b+" https://cache.nixos.org/nix-cache-info"), "egress from A with B's source address")
	mustFail(t, inA(t, "ping -c 2 -W 2 -I "+b+" "+env("HOST_IP")), "ping the host from A with B's source address")
}
