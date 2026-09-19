package app

import (
	"context"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
)

// Aliases keep app.go readable; they are the proto types the stream uses.
type (
	hello     = hostdv1.Hello
	heartbeat = hostdv1.Heartbeat
	command   = hostdv1.Command
)

// streamDialer adapts rotatingDialer to stream.Dialer.
type streamDialer struct{ d *rotatingDialer }

func (s streamDialer) Dial(ctx context.Context) (hostdv1.HostService_SessionClient, func(), error) {
	s.d.mu.Lock()
	inner := s.d.inner
	s.d.mu.Unlock()
	return inner.Dial(ctx)
}
