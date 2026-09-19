// Package hostinfo collects what Register reports about a host: hostname,
// Azure SKU from IMDS (reachable from the host, blocked from guests),
// memory, vCPUs, the running NixOS system, the Cloud Hypervisor version
// and the thin pool size.
package hostinfo

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/lvm"
	"github.com/heracraft/repose/internal/hostd/shell"
)

// IMDSURL is Azure's instance metadata endpoint.
const IMDSURL = "http://169.254.169.254/metadata/instance/compute/vmSize?api-version=2021-02-01&format=text"

// MemInfo returns MemTotal and MemAvailable in bytes from /proc/meminfo.
func MemInfo() (total, avail uint64, err error) {
	return memInfoFrom("/proc/meminfo")
}

func memInfoFrom(path string) (uint64, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }() // read only
	var total, avail uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = kb << 10
		case "MemAvailable:":
			avail = kb << 10
		}
	}
	return total, avail, sc.Err()
}

// Load1 reads the one-minute load average.
func Load1() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

// SKU asks IMDS for the VM size with a short timeout; empty when absent.
func SKU(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, IMDSURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Metadata", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }() // body read below
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Collect builds the HostInfo message.
func Collect(ctx context.Context, r shell.Runner, l lvm.LVM) *hostdv1.HostInfo {
	info := &hostdv1.HostInfo{Vcpus: uint32(runtime.NumCPU())}
	info.Hostname, _ = os.Hostname() // an unreadable hostname is reported as empty
	info.MemBytes, _, _ = MemInfo()  // same
	info.Sku = SKU(ctx)
	if sys, err := os.Readlink("/run/current-system"); err == nil {
		info.NixosSystem = sys
	}
	if res, err := r.Run(ctx, "cloud-hypervisor", "--version"); err == nil {
		info.ChVersion = strings.TrimSpace(string(res.Stdout))
	}
	if l != nil {
		if size, _, err := l.PoolStats(ctx); err == nil {
			info.PoolBytes = size
		}
	}
	return info
}
