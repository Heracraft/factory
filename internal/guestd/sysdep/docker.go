package sysdep

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Docker answers the two questions guestd asks of the daemon: is it alive, and
// how many containers are running. It speaks the Engine API over the unix
// socket rather than forking `docker`, so a sample costs no process.
type Docker interface {
	Ping(ctx context.Context) error
	RunningContainers(ctx context.Context) (int, error)
}

// SocketDocker is the real Docker.
type SocketDocker struct {
	Socket  string
	Timeout time.Duration

	client *http.Client
}

// NewSocketDocker builds a client for the daemon at socket.
func NewSocketDocker(socket string, timeout time.Duration) *SocketDocker {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	d := &SocketDocker{Socket: socket, Timeout: timeout}
	d.client = &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, "unix", d.Socket)
			},
		},
	}
	return d
}

// Ping reports whether the daemon answers.
func (d *SocketDocker) Ping(ctx context.Context) error {
	resp, err := d.get(ctx, "http://docker/_ping")
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // response body of a ping
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker ping: status %d", resp.StatusCode)
	}
	return nil
}

// RunningContainers counts running containers.
func (d *SocketDocker) RunningContainers(ctx context.Context) (int, error) {
	resp, err := d.get(ctx, "http://docker/containers/json")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close() //nolint:errcheck // body is consumed by the decoder
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("docker containers: status %d", resp.StatusCode)
	}
	// Only the element count matters; the ids and names are a tenant's business.
	var list []struct{}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return 0, fmt.Errorf("docker containers: decode: %w", err)
	}
	return len(list), nil
}

func (d *SocketDocker) get(ctx context.Context, url string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, d.Timeout)
	// The cancel runs when the caller closes the body; tying it to the response
	// would leak, so the timeout on the client is the real bound and this
	// context only carries cancellation from above.
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("docker request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker socket: %w", err)
	}
	return resp, nil
}
