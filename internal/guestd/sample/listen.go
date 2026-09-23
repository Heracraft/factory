package sample

import (
	"bytes"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// The guest's listening processes, for `repose status` (DECISIONS I-200):
// which dev servers are still up, how old and how big, so a stale one can
// be found and stopped by hand. Nothing is stopped for the user.
//
// Read without forking: /proc/net/tcp and tcp6 give the listening sockets
// and their inodes; each process's fd links are matched against those
// inodes ("socket:[N]", nothing else is looked at, and no link target
// leaves this function); name, start time and RSS come from its stat.
// Never a command line (DECISIONS R5-3).

// maxListening bounds what one sample carries; a variable for the test that
// runs on a machine with more listeners than a guest has.
var maxListening = 20

// listeningPlatformPorts are the guest's own desktop listeners
// (guest-conventions.md "Desktop"), the same ones the CLI never
// auto-forwards.
var listeningPlatformPorts = map[uint32]bool{6080: true, 6081: true, 5900: true}

// listening returns the forwardable listeners, each with its process when
// one could be found, sorted by port.
func (r *procReader) listening() []*hostdv1.ListeningProc {
	socks := map[uint64]uint32{} // inode -> port
	for _, f := range []string{"tcp", "tcp6"} {
		b, err := os.ReadFile(filepath.Join(r.paths.Proc(), "net", f))
		if err != nil {
			continue
		}
		for inode, port := range parseProcNetTCP(b) {
			socks[inode] = port
		}
	}
	if len(socks) == 0 {
		return nil
	}
	byPort := map[uint32]*hostdv1.ListeningProc{}
	for _, port := range socks {
		byPort[port] = &hostdv1.ListeningProc{Port: port}
	}

	uptime := r.uptimeTicks()
	entries, err := os.ReadDir(r.paths.Proc())
	if err == nil {
		left := len(socks)
		for _, e := range entries {
			if left == 0 {
				break
			}
			if !isPID(e.Name()) {
				continue
			}
			fdDir := filepath.Join(r.paths.ProcPID(e.Name()), "fd")
			fds, err := os.ReadDir(fdDir)
			if err != nil {
				continue // not ours to read, or gone
			}
			for _, fd := range fds {
				target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
				if err != nil || !strings.HasPrefix(target, "socket:[") {
					continue
				}
				inode, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]"), 10, 64)
				if err != nil {
					continue
				}
				port, ok := socks[inode]
				if !ok {
					continue
				}
				delete(socks, inode)
				left--
				lp := byPort[port]
				if lp.Comm != "" {
					continue // v4 and v6 sockets of one server
				}
				comm, start, rss, ok := r.readStartAndRSS(e.Name())
				if !ok {
					continue
				}
				lp.Comm, lp.RssBytes = comm, rss
				if uptime > start {
					lp.AgeSeconds = (uptime - start) / clockTicks
				}
			}
		}
	}
	out := make([]*hostdv1.ListeningProc, 0, len(byPort))
	for _, lp := range byPort {
		out = append(out, lp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	if len(out) > maxListening {
		out = out[:maxListening]
	}
	return out
}

// parseProcNetTCP picks the listening sockets on a loopback or wildcard
// address, port 1024 and up, not the desktop's, from /proc/net/tcp(6):
// inode -> port.
func parseProcNetTCP(b []byte) map[uint64]uint32 {
	out := map[uint64]uint32{}
	for i, line := range bytes.Split(b, []byte("\n")) {
		f := strings.Fields(string(line))
		if i == 0 || len(f) < 10 || f[3] != "0A" { // 0A is TCP_LISTEN
			continue
		}
		addr, portHex, ok := strings.Cut(f[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil || port < 1024 || listeningPlatformPorts[uint32(port)] {
			continue
		}
		ip := procNetIP(addr)
		if ip == nil || (!ip.IsLoopback() && !ip.IsUnspecified()) {
			continue
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		out[inode] = uint32(port)
	}
	return out
}

// procNetIP decodes /proc/net's address: 8 hex digits for IPv4, 32 for
// IPv6, each 32-bit word in host (little-endian) order.
func procNetIP(s string) net.IP {
	b, err := hex.DecodeString(s)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return nil
	}
	for i := 0; i+4 <= len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	ip := net.IP(b)
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// readStartAndRSS reads comm, starttime (field 22, clock ticks since
// boot) and RSS from a process's stat.
func (r *procReader) readStartAndRSS(pid string) (comm string, start, rss uint64, ok bool) {
	b, err := os.ReadFile(filepath.Join(r.paths.ProcPID(pid), "stat"))
	if err != nil {
		return "", 0, 0, false
	}
	nameStart := bytes.IndexByte(b, '(')
	nameEnd := bytes.LastIndexByte(b, ')')
	if nameStart < 0 || nameEnd < nameStart {
		return "", 0, 0, false
	}
	rest := bytes.Fields(b[nameEnd+1:])
	const (
		startIdx = 19 // field 22
		rssIdx   = 21 // field 24
	)
	if len(rest) <= rssIdx {
		return "", 0, 0, false
	}
	start, err1 := strconv.ParseUint(string(rest[startIdx]), 10, 64)
	pages, err2 := strconv.ParseUint(string(rest[rssIdx]), 10, 64)
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}
	return string(b[nameStart+1 : nameEnd]), start, pages * pageSize, true
}

// uptimeTicks is /proc/uptime in clock ticks.
func (r *procReader) uptimeTicks() uint64 {
	b, err := os.ReadFile(r.paths.Uptime())
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0
	}
	return uint64(secs * clockTicks)
}
