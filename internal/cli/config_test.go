package cli

import (
	"os"
	"slices"
	"testing"
)

// sync.exclude is a [sync] table in TOML; the quoted "sync.exclude" key
// is the spelling older configs used. Both must reach the sync.
func TestLoadConfigSyncExclude(t *testing.T) {
	cases := map[string][]string{
		"[sync]\nexclude = [\"dist\", \"*.mp4\"]\n": {"dist", "*.mp4"},
		"\"sync.exclude\" = [\"dist\"]\n":           {"dist"},
		"default_class = \"small\"\n":               nil,
	}
	for body, want := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(configPath(dir), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := loadConfig(dir)
		if err != nil {
			t.Fatalf("%q: %v", body, err)
		}
		if !slices.Equal(cfg.SyncExclude, want) {
			t.Errorf("%q: SyncExclude = %v, want %v", body, cfg.SyncExclude, want)
		}
	}
}
