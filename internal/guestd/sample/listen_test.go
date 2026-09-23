//go:build linux

package sample

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/heracraft/repose/internal/guestd/sysdep"
)

func TestParseProcNetTCP(t *testing.T) {
	tcp := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:1435 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 111 1 0000000000000000 100 0 0 10 0
   1: 00000000:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 222 1 0000000000000000 100 0 0 10 0
   2: 0100007F:17C0 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 333 1 0000000000000000 100 0 0 10 0
   3: 050040A0:1B58 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 444 1 0000000000000000 100 0 0 10 0
   4: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 555 1 0000000000000000 100 0 0 10 0
   5: 0100007F:1435 0100007F:C350 01 00000000:00000000 00:00000000 00000000  1000        0 666 1 0000000000000000 100 0 0 10 0
`
	got := parseProcNetTCP([]byte(tcp))
	want := map[uint64]uint32{111: 5173, 222: 3000}
	if len(got) != len(want) || got[111] != 5173 || got[222] != 3000 {
		t.Errorf("tcp = %v, want %v (6080 is the desktop's, 10.64.0.5 and :22 are not forwardable, 666 is not listening)", got, want)
	}
	tcp6 := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:1435 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 777 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000000000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 888 1 0000000000000000 100 0 0 10 0
`
	got = parseProcNetTCP([]byte(tcp6))
	if got[777] != 5173 || got[888] != 8080 || len(got) != 2 {
		t.Errorf("tcp6 = %v", got)
	}
}

// Against this machine's real /proc: a listener this test opens is found
// with this process's name, a plausible age and its memory.
func TestListeningFindsARealListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	r := newProcReader(sysdep.Paths{Root: "/"})
	prev := maxListening
	maxListening = 1 << 20
	defer func() { maxListening = prev }()
	comm := filepath.Base(os.Args[0])
	if len(comm) > 15 {
		comm = comm[:15]
	}
	for _, lp := range r.listening() {
		if lp.Port != port {
			continue
		}
		if lp.Comm != comm || lp.RssBytes == 0 || lp.AgeSeconds > 3600 {
			t.Fatalf("listener = %+v, want comm %q with memory and a young age", lp, comm)
		}
		return
	}
	t.Fatalf("port %d not listed", port)
}
