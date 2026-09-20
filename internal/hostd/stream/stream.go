// Package stream holds the Session stream to the api: dial with the host
// certificate, Hello from bbolt, heartbeats every 15 s, commands down to
// the Manager, results, events, samples and build logs up. Any error
// reconnects with backoff 1, 2, 4, 8, 16, 30, 30... seconds plus jitter.
// Samples are buffered for 60 minutes across an outage; events are kept
// until the api acks them.
package stream

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	hostdv1 "github.com/heracraft/repose/internal/gen/hostd/v1"
	"github.com/heracraft/repose/internal/hostd/metrics"
	"github.com/heracraft/repose/internal/obs"
)

// Host is what the stream needs from the guest Manager.
type Host interface {
	Hello() *hostdv1.Hello
	Heartbeat() *hostdv1.Heartbeat
	Dispatch(cmd *hostdv1.Command)
}

// Dialer opens a Session; the default dials gRPC over mTLS, tests use an
// in-process server.
type Dialer interface {
	Dial(ctx context.Context) (hostdv1.HostService_SessionClient, func(), error)
}

// GRPCDialer dials the api with the host's client certificate.
type GRPCDialer struct {
	Addr string
	TLS  *tls.Config
}

// Dial implements Dialer.
func (d GRPCDialer) Dial(ctx context.Context) (hostdv1.HostService_SessionClient, func(), error) {
	conn, err := grpc.NewClient(d.Addr,
		grpc.WithTransportCredentials(credentials.NewTLS(d.TLS)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 30 * time.Second, Timeout: 10 * time.Second, PermitWithoutStream: true}),
	)
	if err != nil {
		return nil, nil, err
	}
	s, err := hostdv1.NewHostServiceClient(conn).Session(ctx)
	if err != nil {
		_ = conn.Close() // the dial error is what matters
		return nil, nil, err
	}
	return s, func() { _ = conn.Close() }, nil // closing a connection we are abandoning; nothing to report
}

// Config tunes the stream.
type Config struct {
	HeartbeatInterval time.Duration
	SampleBuffer      int
	MaxBackoff        time.Duration
	// Backoff overrides the schedule (tests).
	BackoffBase time.Duration
}

// Stream is the running connection manager. It implements guest.Emitter.
type Stream struct {
	cfg     Config
	dialer  Dialer
	host    Host
	metrics *metrics.M
	log     *slog.Logger

	out       chan *hostdv1.HostMessage
	mu        sync.Mutex
	events    map[string]*hostdv1.Event
	eventList []string
	samples   []*hostdv1.Samples
	kick      chan struct{}
	connected bool
	reconnect chan struct{}
	dropped   uint64
}

// New builds a Stream.
func New(cfg Config, dialer Dialer, host Host, m *metrics.M, log *slog.Logger) *Stream {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 15 * time.Second
	}
	if cfg.SampleBuffer == 0 {
		cfg.SampleBuffer = 60
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.BackoffBase == 0 {
		cfg.BackoffBase = time.Second
	}
	if log == nil {
		log = obs.Nop(obs.ComponentHostd)
	}
	return &Stream{
		cfg: cfg, dialer: dialer, host: host, metrics: m, log: log,
		out: make(chan *hostdv1.HostMessage, 8192), events: map[string]*hostdv1.Event{},
		kick: make(chan struct{}, 1), reconnect: make(chan struct{}, 1),
	}
}

// Connected reports whether a session is open.
func (s *Stream) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

// Reconnect drops the current session (certificate rotation).
func (s *Stream) Reconnect() {
	select {
	case s.reconnect <- struct{}{}:
	default:
	}
}

// Result implements guest.Emitter. Results are already in bbolt; a send
// that never happens is replayed on the api's request after Hello.
func (s *Stream) Result(res *hostdv1.Result) {
	s.out <- &hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Result{Result: res}}
}

// Event implements guest.Emitter: kept until acked.
func (s *Stream) Event(ev *hostdv1.Event) {
	s.mu.Lock()
	s.events[ev.EventId] = ev
	s.eventList = append(s.eventList, ev.EventId)
	if len(s.eventList) > 10000 {
		old := s.eventList[0]
		s.eventList = s.eventList[1:]
		delete(s.events, old)
	}
	if s.metrics != nil {
		s.metrics.EventsPending.Set(float64(len(s.events)))
	}
	s.mu.Unlock()
	s.out <- &hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Event{Event: ev}}
}

// Samples implements guest.Emitter: buffered, oldest dropped past the cap.
func (s *Stream) Samples(sm *hostdv1.Samples) {
	s.mu.Lock()
	s.samples = append(s.samples, sm)
	for len(s.samples) > s.cfg.SampleBuffer {
		s.samples = s.samples[1:]
		s.dropped++
		if s.metrics != nil {
			s.metrics.SamplesDroppedTotal.Inc()
		}
	}
	s.mu.Unlock()
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// BuildLog implements guest.Emitter; a full queue drops the line rather
// than stalling the build.
func (s *Stream) BuildLog(commandID string, seq uint64, line string) {
	msg := &hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Log{Log: &hostdv1.BuildLog{CommandId: commandID, Seq: seq, Line: line}}}
	select {
	case s.out <- msg:
	default:
	}
}

// Dropped is the count of samples dropped from the buffer.
func (s *Stream) Dropped() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropped
}

func (s *Stream) setConnected(v bool) {
	s.mu.Lock()
	s.connected = v
	s.mu.Unlock()
	if s.metrics != nil {
		if v {
			s.metrics.StreamConnected.Set(1)
		} else {
			s.metrics.StreamConnected.Set(0)
		}
	}
}

// Run keeps a session open until ctx ends.
func (s *Stream) Run(ctx context.Context) {
	attempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		err := s.session(ctx)
		s.setConnected(false)
		if ctx.Err() != nil {
			return
		}
		if s.metrics != nil {
			s.metrics.StreamReconnectsTotal.Inc()
		}
		d := s.backoff(attempt)
		attempt++
		if err != nil {
			s.log.Warn("stream disconnected", "event", "stream_disconnect", "err", err.Error(), "retry_ms", d.Milliseconds())
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(d):
		}
	}
}

func (s *Stream) backoff(attempt int) time.Duration {
	d := s.cfg.BackoffBase << uint(attempt)
	if attempt > 10 || d > s.cfg.MaxBackoff {
		d = s.cfg.MaxBackoff
	}
	jitter := time.Duration(rand.Int63n(int64(d)/4 + 1))
	return d + jitter
}

func (s *Stream) session(ctx context.Context) error {
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sess, closeFn, err := s.dialer.Dial(sctx)
	if err != nil {
		return err
	}
	defer closeFn()
	if err := sess.Send(&hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Hello{Hello: s.host.Hello()}}); err != nil {
		return err
	}
	s.setConnected(true)
	s.log.Info("stream connected", "event", "stream_connect")

	var wg sync.WaitGroup
	sendErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		sendErr <- s.sender(sctx, sess)
	}()
	recvErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		recvErr <- s.receiver(sctx, sess)
	}()
	select {
	case err = <-sendErr:
	case err = <-recvErr:
	case <-s.reconnect:
		err = errors.New("reconnect requested")
	case <-ctx.Done():
		err = nil
	}
	cancel()
	wg.Wait()
	return err
}

func (s *Stream) sender(ctx context.Context, sess hostdv1.HostService_SessionClient) error {
	// After Hello: pending events, then buffered samples.
	s.mu.Lock()
	pending := make([]*hostdv1.Event, 0, len(s.events))
	for _, id := range s.eventList {
		if ev, ok := s.events[id]; ok {
			pending = append(pending, ev)
		}
	}
	s.mu.Unlock()
	for _, ev := range pending {
		if err := sess.Send(&hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Event{Event: ev}}); err != nil {
			return err
		}
	}
	if err := s.flushSamples(sess); err != nil {
		return err
	}
	hb := time.NewTicker(s.cfg.HeartbeatInterval)
	defer hb.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-hb.C:
			if err := sess.Send(&hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Heartbeat{Heartbeat: s.host.Heartbeat()}}); err != nil {
				return err
			}
		case <-s.kick:
			if err := s.flushSamples(sess); err != nil {
				return err
			}
		case m := <-s.out:
			if ev, ok := m.Msg.(*hostdv1.HostMessage_Event); ok {
				s.mu.Lock()
				_, still := s.events[ev.Event.EventId]
				s.mu.Unlock()
				if !still {
					continue // acked already (sent on a previous session)
				}
			}
			if err := sess.Send(m); err != nil {
				return err
			}
		}
	}
}

func (s *Stream) flushSamples(sess hostdv1.HostService_SessionClient) error {
	for {
		s.mu.Lock()
		if len(s.samples) == 0 {
			s.mu.Unlock()
			return nil
		}
		sm := s.samples[0]
		s.mu.Unlock()
		if err := sess.Send(&hostdv1.HostMessage{Msg: &hostdv1.HostMessage_Samples{Samples: sm}}); err != nil {
			return err
		}
		s.mu.Lock()
		if len(s.samples) > 0 && s.samples[0] == sm {
			s.samples = s.samples[1:]
		}
		s.mu.Unlock()
	}
}

func (s *Stream) receiver(ctx context.Context, sess hostdv1.HostService_SessionClient) error {
	for {
		msg, err := sess.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("api closed the stream")
			}
			return err
		}
		switch m := msg.Msg.(type) {
		case *hostdv1.ApiMessage_Command:
			if m.Command != nil {
				s.host.Dispatch(m.Command)
			}
		case *hostdv1.ApiMessage_Ack:
			s.mu.Lock()
			delete(s.events, m.Ack.EventId)
			if s.metrics != nil {
				s.metrics.EventsPending.Set(float64(len(s.events)))
			}
			s.mu.Unlock()
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}
