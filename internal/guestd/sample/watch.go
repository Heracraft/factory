package sample

import "github.com/heracraft/repose/internal/abuse"

// Watched process names are always reported in a sample even when they are
// not in the top slice by CPU, so a miner that throttles itself to stay off
// the top of the list still shows up. The list is mirrored in
// docs/SECURITY.md; add to both or to neither.
//
// A name on this list is not an accusation. It is a name worth seeing.
var watched = map[string]bool{
	"xmrig":        true,
	"minerd":       true,
	"cpuminer":     true,
	"ccminer":      true,
	"cgminer":      true,
	"bfgminer":     true,
	"ethminer":     true,
	"nbminer":      true,
	"phoenixminer": true,
	"t-rex":        true,
	"lolminer":     true,
	"xmr-stak":     true,
	"kdevtmpfsi":   true,
	"kinsing":      true,
	"tsm":          true,
	"sysrv":        true,
	"masscan":      true,
	"zmap":         true,
	"hashcat":      true,
	"john":         true,
}

// Watched reports whether comm is on the watch list or is a miner the api
// stops a guest for (internal/abuse, DECISIONS I-239), in any case and with
// any suffix (lolMiner, xmrig-notls), so a throttled miner below the top
// slice still reaches the api.
func Watched(comm string) bool {
	if watched[comm] {
		return true
	}
	_, miner := abuse.MinerName(comm)
	return miner
}

// WatchList returns the names, for the SECURITY.md cross-check test.
func WatchList() []string {
	out := make([]string, 0, len(watched))
	for name := range watched {
		out = append(out, name)
	}
	return out
}
