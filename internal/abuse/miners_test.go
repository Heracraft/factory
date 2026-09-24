package abuse

import "testing"

func TestMinerName(t *testing.T) {
	for comm, want := range map[string]string{
		"xmrig":           "xmrig",
		"XMRig":           "xmrig",
		"xmrig-notls":     "xmrig-notls",
		"xmr-stak-rx":     "xmr-stak-rx",
		"SRBMiner-MULTI":  "srbminer-multi",
		"lolMiner":        "lolminer",
		"PhoenixMiner":    "phoenixminer",
		"cpuminer-opt":    "cpuminer-opt",
		"t-rex":           "t-rex",
		"nanominer":       "nanominer",
		"teamredminer":    "teamredminer",
		"bzminer":         "bzminer",
		"rigel":           "rigel",
		"onezerominer":    "onezerominer",
		"kdevtmpfsi":      "kdevtmpfsi",
		"minerd":          "minerd",
		"ccminer":         "ccminer",
		"gminer":          "gminer",
		"ethminer":        "ethminer",
		"nbminer":         "nbminer",
		" xmrig ":         "xmrig",
		"verthashminer-x": "verthashminer-x",
	} {
		got, ok := MinerName(comm)
		if !ok {
			t.Errorf("MinerName(%q) not a miner; want %q", comm, want)
			continue
		}
		if got != want {
			t.Errorf("MinerName(%q) = %q, want %q", comm, got, want)
		}
	}
	// Developer tools and names that only look close.
	for _, comm := range []string{
		"", "node", "miner", "minikube", "minizinc", "rigel-server", "xmllint", "xmr-wallet-rpc2", "trex", "tsserver",
		"cargo", "rustc", "go", "python3", "java", "claude", "code", "docker", "containerd", "postgres", "hashcat", "john",
		"masscan", "npm", "pnpm", "esbuild", "gopls", "rust-analyzer", "chromium", "playwright",
	} {
		if n, ok := MinerName(comm); ok {
			t.Errorf("MinerName(%q) = %q; not a miner", comm, n)
		}
	}
	if len(MinerList()) < 30 {
		t.Fatalf("MinerList has %d entries", len(MinerList()))
	}
}
