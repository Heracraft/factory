package hostinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemInfo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "meminfo")
	_ = os.WriteFile(p, []byte("MemTotal:       65536000 kB\nMemFree:         1000 kB\nMemAvailable:   40000000 kB\n"), 0o644)
	total, avail, err := memInfoFrom(p)
	if err != nil || total != 65536000<<10 || avail != 40000000<<10 {
		t.Fatalf("%d %d %v", total, avail, err)
	}
}
