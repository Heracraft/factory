package vsockrpc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/mdlayher/vsock"
)

// Port is the vsock port guestd listens on (docs/interfaces/vsock-guestd.md).
const Port uint32 = 5000

// Listen listens on the guest side of vsock, on any CID. Binding
// VMADDR_CID_HOST (2) inside a guest fails with EADDRNOTAVAIL, which is what
// the first guest on host-01 logged every two seconds (DECISIONS I-52);
// vsock.Listen binds VMADDR_CID_ANY.
func Listen(port uint32) (net.Listener, error) {
	l, err := vsock.Listen(port, nil)
	if err != nil {
		return nil, fmt.Errorf("listen vsock port %d: %w", port, err)
	}
	return l, nil
}

// ListenUnix serves the same protocol on a unix socket, for the dev mode and
// for tests on machines without vsock. The socket is replaced if a stale one
// is present, and created with mode 0600 (only hostd, running as root, dials
// it; the hook socket is the one the dev user touches).
func ListenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create socket directory: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = l.Close()
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return l, nil
}

// Dial is hostd's side: connect to a guest's vsock CID.
func Dial(cid, port uint32) (net.Conn, error) {
	c, err := vsock.Dial(cid, port, nil)
	if err != nil {
		return nil, fmt.Errorf("dial vsock cid %d port %d: %w", cid, port, err)
	}
	return c, nil
}

// DialUnix is hostd's side in dev mode.
func DialUnix(path string) (net.Conn, error) {
	c, err := net.Dial("unix", path)
	if err != nil {
		return nil, fmt.Errorf("dial unix %s: %w", filepath.Base(path), err)
	}
	return c, nil
}
