// Package ch renders the Cloud Hypervisor invocation for a guest and talks
// to its API socket for shutdown, pause, resume and info.
package ch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Spec is everything the argv needs; the guest package fills it from the
// guest record and the system closure.
type Spec struct {
	GuestDir     string // /var/lib/repose/guests/<id>
	Kernel       string // <closure>/kernel
	Initrd       string // <closure>/initrd
	Init         string // <closure>/init
	KernelParams string // contents of <closure>/kernel-params
	IP           string
	Gateway      string
	Netmask      string
	Tap          string
	MAC          string
	CID          uint32
	VolumeDev    string
	VCPUs        uint32
	MemMiB       uint64
	StoreTag     string // ro-store
}

// Paths under the guest directory.
func APISocket(dir string) string      { return filepath.Join(dir, "ch.sock") }
func VsockSocket(dir string) string    { return filepath.Join(dir, "vsock.sock") }
func ConsoleSocket(dir string) string  { return filepath.Join(dir, "console.sock") }
func VirtiofsSocket(dir string) string { return filepath.Join(dir, "virtiofsd.sock") }

// Cmdline renders the kernel command line: the closure's init and params,
// the serial console, and the static address the guest's networkd reads.
func (s Spec) Cmdline() string {
	parts := []string{"init=" + s.Init}
	if p := strings.TrimSpace(s.KernelParams); p != "" {
		parts = append(parts, p)
	}
	parts = append(parts, "console=ttyS0",
		fmt.Sprintf("ip=%s::%s:%s::eth0:off", s.IP, s.Gateway, s.Netmask))
	return strings.Join(parts, " ")
}

// Args renders the cloud-hypervisor argv.
func (s Spec) Args() []string {
	return []string{
		"cloud-hypervisor",
		"--api-socket", APISocket(s.GuestDir),
		"--kernel", s.Kernel,
		"--initramfs", s.Initrd,
		"--cmdline", s.Cmdline(),
		"--cpus", fmt.Sprintf("boot=%d", s.VCPUs),
		"--memory", fmt.Sprintf("size=%dM,shared=on", s.MemMiB),
		"--disk", "path=" + s.VolumeDev,
		"--net", fmt.Sprintf("tap=%s,mac=%s", s.Tap, s.MAC),
		"--fs", fmt.Sprintf("tag=%s,socket=%s", s.StoreTag, VirtiofsSocket(s.GuestDir)),
		"--vsock", fmt.Sprintf("cid=%d,socket=%s", s.CID, VsockSocket(s.GuestDir)),
		"--serial", "socket=" + ConsoleSocket(s.GuestDir),
		"--console", "off",
	}
}

// Client is the API-socket side.
type Client interface {
	Shutdown(ctx context.Context, sock string) error
	Pause(ctx context.Context, sock string) error
	Resume(ctx context.Context, sock string) error
	Info(ctx context.Context, sock string) (json.RawMessage, error)
	Version(ctx context.Context) (string, error)
}

// HTTP talks to the API socket over HTTP/unix.
type HTTP struct {
	Timeout time.Duration
	// VersionArgv is what prints the version, usually cloud-hypervisor --version.
	VersionFn func(ctx context.Context) (string, error)
}

func (h *HTTP) client(sock string) *http.Client {
	t := h.Timeout
	if t == 0 {
		t = 10 * time.Second
	}
	return &http.Client{Timeout: t, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}}
}

func (h *HTTP) put(ctx context.Context, sock, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://localhost/api/v1/"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client(sock).Do(req)
	if err != nil {
		return nil, fmt.Errorf("ch api %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()                // body drained below
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // error bodies are informational only
	if resp.StatusCode >= 300 {
		return body, fmt.Errorf("ch api %s: status %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (h *HTTP) get(ctx context.Context, sock, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/api/v1/"+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client(sock).Do(req)
	if err != nil {
		return nil, fmt.Errorf("ch api %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }() // body read below
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return body, fmt.Errorf("ch api %s: status %d", path, resp.StatusCode)
	}
	return body, nil
}

func (h *HTTP) Shutdown(ctx context.Context, sock string) error {
	_, err := h.put(ctx, sock, "vm.shutdown")
	return err
}

func (h *HTTP) Pause(ctx context.Context, sock string) error {
	_, err := h.put(ctx, sock, "vm.pause")
	return err
}

func (h *HTTP) Resume(ctx context.Context, sock string) error {
	_, err := h.put(ctx, sock, "vm.resume")
	return err
}

func (h *HTTP) Info(ctx context.Context, sock string) (json.RawMessage, error) {
	b, err := h.get(ctx, sock, "vm.info")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func (h *HTTP) Version(ctx context.Context) (string, error) {
	if h.VersionFn == nil {
		return "", nil
	}
	return h.VersionFn(ctx)
}

// Fake records API calls.
type Fake struct {
	mu    sync.Mutex
	Calls []string
	Fail  map[string]error
	Ver   string
}

func (f *Fake) record(op, sock string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, op+" "+sock)
	return f.Fail[op]
}

func (f *Fake) Shutdown(_ context.Context, sock string) error { return f.record("shutdown", sock) }
func (f *Fake) Pause(_ context.Context, sock string) error    { return f.record("pause", sock) }
func (f *Fake) Resume(_ context.Context, sock string) error   { return f.record("resume", sock) }
func (f *Fake) Info(_ context.Context, sock string) (json.RawMessage, error) {
	if err := f.record("info", sock); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"state":"Running"}`), nil
}
func (f *Fake) Version(context.Context) (string, error) { return f.Ver, nil }
