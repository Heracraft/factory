package questions

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newStore(t *testing.T) (*Store, *clock, *[]Question) {
	t.Helper()
	c := &clock{t: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	var mu sync.Mutex
	var emitted []Question
	s := New(t.TempDir(), func(q Question) { mu.Lock(); emitted = append(emitted, q); mu.Unlock() },
		slog.New(slog.NewTextHandler(io.Discard, nil)), c.now)
	return s, c, &emitted
}

func TestOpenValidatesAndCaps(t *testing.T) {
	s, _, emitted := newStore(t)
	if _, err := s.Open("claude", "w", "   ", nil, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty text: %v", err)
	}
	if _, err := s.Open("claude", "w", "q", []string{"a", "b", "c", "d"}, 0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("four options: %v", err)
	}
	q, err := s.Open("claude", "w", strings.Repeat("é", 1000), []string{strings.Repeat("x", 100), "no", "No"}, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Text) > TextCap || !strings.HasPrefix(q.Text, "é") {
		t.Fatalf("text not capped on a rune boundary: %d bytes", len(q.Text))
	}
	if len(q.Options) != 2 || len(q.Options[0]) != OptionCap {
		t.Fatalf("options = %q", q.Options)
	}
	if q.TimeoutS != uint32(MaxTimeout/time.Second) {
		t.Fatalf("timeout not capped: %d", q.TimeoutS)
	}
	if len(*emitted) != 1 {
		t.Fatalf("emitted %d", len(*emitted))
	}
	for i := 1; i < MaxOpen; i++ {
		if _, err := s.Open("claude", "w", "q", nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Open("claude", "w", "one too many", nil, 0); !errors.Is(err, ErrTooMany) {
		t.Fatalf("seventeenth: %v", err)
	}
}

func TestSweepExpiresAndForgets(t *testing.T) {
	s, c, emitted := newStore(t)
	q, _ := s.Open("pi", "pi", "still there?", nil, time.Minute)
	c.add(61 * time.Second)
	s.Sweep()
	got, err := s.Get(q.ID)
	if err != nil || got.State != StateExpired {
		t.Fatalf("after the timeout: %+v %v", got, err)
	}
	if last := (*emitted)[len(*emitted)-1]; last.State != StateExpired {
		t.Fatalf("expiry not announced: %+v", last)
	}
	// The answer that arrives after expiry changes nothing.
	if err := s.Answer(q.ID, StateAnswered, "yes"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(q.ID); got.State != StateExpired || got.Answer != "" {
		t.Fatalf("late answer applied: %+v", got)
	}
	c.add(Keep + time.Second)
	s.Sweep()
	if _, err := s.Get(q.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("closed question kept past Keep: %v", err)
	}
}

func TestWaitReturnsOnCloseOrDeadline(t *testing.T) {
	s, _, _ := newStore(t)
	q, _ := s.Open("claude", "w", "q", nil, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if got, _ := s.Wait(ctx, q.ID); got.State != StateOpen {
		t.Fatalf("wait past the deadline = %+v", got)
	}
	go func() { time.Sleep(10 * time.Millisecond); _ = s.Answer(q.ID, StateNoChannel, "") }()
	if got, _ := s.Wait(context.Background(), q.ID); got.State != StateNoChannel {
		t.Fatalf("wait = %+v", got)
	}
	if err := s.Answer(q.ID, "bogus", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bogus status: %v", err)
	}
}
