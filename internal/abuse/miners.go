// Package abuse holds what the platform treats as abuse by name alone
// (DECISIONS I-239): the cryptocurrency miners whose process name, as
// guestd samples it, gets a guest stopped. The api acts on it; guestd uses
// it to keep a miner in a sample even below the top of the CPU list. Both
// read the process name only, never arguments, which is the boundary the
// privacy policy promises (docs/ops/OBSERVABILITY.md).
package abuse

import (
	"sort"
	"strings"
)

// minerPrefixes match a process name that starts with them, compared in
// lower case: a miner's forks and builds keep the name and add a suffix
// (xmrig-notls, cpuminer-opt, SRBMiner-MULTI, xmr-stak-rx, lolMiner). Each
// is a mining program's own name, long enough that no common developer
// tool starts with it.
var minerPrefixes = []string{
	"xmrig", "xmr-stak", "cpuminer", "minerd", "ccminer", "cgminer", "bfgminer", "sgminer",
	"t-rex", "nbminer", "lolminer", "gminer", "teamredminer", "phoenixminer", "ethminer",
	"nanominer", "srbminer", "bzminer", "onezerominer", "wildrig", "kawpowminer",
	"nheqminer", "hellminer", "cryptodredge", "z-enemy", "claymore",
	"ethdcrminer", "nsfminer", "verthashminer", "danila-miner",
}

// minerNames match exactly, in lower case: names too short or too generic
// to use as a prefix, and the names of mining malware seen on rented
// machines.
var minerNames = map[string]bool{
	"kdevtmpfsi": true, // kinsing's miner
	"kinsing":    true,
	"sysrv":      true,
	"xmr":        true,
	"xmrminer":   true,
	"rigel":      true,
	"miniz":      true, // miniZ; as a prefix it would take minizinc
}

// MinerName reports whether comm, a process name as /proc/<pid>/stat gives
// it (at most 15 bytes), names a known miner, and the lower-case name to
// show for it.
func MinerName(comm string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(comm))
	if c == "" {
		return "", false
	}
	if minerNames[c] {
		return c, true
	}
	for _, p := range minerPrefixes {
		if strings.HasPrefix(c, p) {
			return c, true
		}
	}
	return "", false
}

// MinerList is every name and prefix, sorted, for the docs cross-check.
func MinerList() []string {
	out := append([]string(nil), minerPrefixes...)
	for n := range minerNames {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
