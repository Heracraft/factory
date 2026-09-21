package isolation

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// bench times a fixed CPU job in B and returns the best of n runs in seconds.
func bench(t *testing.T, n int) float64 {
	t.Helper()
	best := 0.0
	for i := 0; i < n; i++ {
		// Milliseconds through shell arithmetic: a guest has coreutils and
		// sh, not necessarily bc.
		r := inB(t, "s=$(date +%s%N); head -c 268435456 /dev/zero | sha256sum >/dev/null; e=$(date +%s%N); echo $(( (e - s) / 1000000 ))")
		mustSucceed(t, r, "benchmark in B")
		ms, err := strconv.ParseFloat(strings.TrimSpace(r.out), 64)
		if err != nil {
			t.Fatalf("benchmark output %q: %v", r.out, err)
		}
		v := ms / 1000
		if best == 0 || v < best {
			best = v
		}
	}
	return best
}

// Row: guest cannot escape memory or CPU limits. A fork bomb and a memory
// hog in A, bounded by systemd so the test can end, must leave B's
// benchmark within 10 percent of its baseline.
func TestForkBombAndMemoryHogLeaveNeighbourWithinTenPercent(t *testing.T) {
	need(t, "EXEC_A", "EXEC_B")
	if r := inB(t, "command -v sha256sum"); r.code != 0 {
		t.Skip("B needs sha256sum for the benchmark")
	}
	baseline := bench(t, 3)
	t.Logf("baseline in B: %.3fs", baseline)

	stop := func() {
		inA(t, "sudo -n systemctl stop repose-iso-forkbomb.service repose-iso-memhog.service 2>/dev/null; true")
	}
	defer stop()
	// A real fork bomb, in a unit with a task cap and a lifetime; the cap is
	// far above what a guest needs so the bomb hits the guest's own limits.
	mustSucceed(t, inA(t, "sudo -n systemd-run --quiet --unit repose-iso-forkbomb -p RuntimeMaxSec=120 -p TasksMax=8000 sh -c ':(){ :|:& };:'"), "start fork bomb in A")
	mem := "python3 -c 'import time\nb=[]\ntry:\n  while True: b.append(bytearray(64<<20))\nexcept MemoryError:\n  pass\ntime.sleep(120)'"
	mustSucceed(t, inA(t, "sudo -n systemd-run --quiet --unit repose-iso-memhog -p RuntimeMaxSec=120 "+mem), "start memory hog in A")
	time.Sleep(10 * time.Second)

	during := bench(t, 3)
	t.Logf("during A's fork bomb and memory hog, B: %.3fs (baseline %.3fs, ratio %.2f)", during, baseline, during/baseline)
	stop()
	if during > baseline*1.10 {
		t.Fatalf("neighbour slowed by %.0f%%, more than 10%%", (during/baseline-1)*100)
	}
	// And A itself is still answering: the limits contained the bomb.
	mustSucceed(t, inA(t, "true"), "A still reachable after the bomb")
}
