// Package questions holds the questions `repose-ask` is waiting on inside a
// guest (DECISIONS I-244). A question is opened from the hook socket, sent
// up to hostd as a Question notify, and closed by hostd's AnswerQuestion
// request, by the asker giving up, or by its own timeout.
//
// Each open question is a file under /run/repose/questions, so a guestd
// restart keeps every question an asker is still waiting on; a reboot
// clears the tmpfs, and an asker that reconnects after one gets not found.
// The question and answer text are tenant content: they go to hostd and
// into these root-only files, and never into a log line.
package questions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limits (docs/interfaces/guest-conventions.md "Asking the user").
const (
	// TextCap is the cap on a question's text and on a notify message, the
	// same 1 KB as an AgentEvent summary.
	TextCap = 1 << 10
	// AnswerCap is the cap on an answer.
	AnswerCap = 1 << 10
	// MaxOptions is how many fixed answers a question may offer: ntfy
	// shows at most three action buttons.
	MaxOptions = 3
	// OptionCap is the cap on one option.
	OptionCap = 64
	// MaxOpen is how many questions one guest may have open at once.
	MaxOpen = 16
	// DefaultTimeout is how long an ask waits without --timeout.
	DefaultTimeout = 30 * time.Minute
	// MaxTimeout is the longest an ask may wait.
	MaxTimeout = 24 * time.Hour
	// Keep is how long a closed question stays readable, so an asker that
	// was reconnecting when the answer arrived still gets it.
	Keep = 10 * time.Minute
)

// States. "" (open) is never written as a status; the rest are the status
// values of the AnswerQuestion request and the Question notify.
const (
	StateOpen      = "open"
	StateAnswered  = "answered"
	StateCancelled = "cancelled"
	StateExpired   = "expired"
	StateNoChannel = "no_channel"
)

// Errors.
var (
	ErrNotFound = errors.New("questions: no such question")
	ErrTooMany  = errors.New("questions: too many open questions")
	ErrInvalid  = errors.New("questions: invalid question")
)

// Question is one ask.
type Question struct {
	ID       string    `json:"id"`
	Agent    string    `json:"agent"`
	Window   string    `json:"window,omitempty"`
	Text     string    `json:"text"`
	Options  []string  `json:"options,omitempty"`
	TimeoutS uint32    `json:"timeout_s"`
	Created  time.Time `json:"created"`
	Expires  time.Time `json:"expires"`
	State    string    `json:"state"`
	Answer   string    `json:"answer,omitempty"`
	Closed   time.Time `json:"closed,omitempty"`
}

// Open reports whether the question is still waiting.
func (q Question) Open() bool { return q.State == StateOpen }

// Emitter is told about a question when it opens, when it is re-announced
// on a new hostd connection, and when the guest side closes it.
type Emitter func(q Question)

type entry struct {
	q    Question
	done chan struct{}
}

// Store is the set of questions of this guest.
type Store struct {
	dir  string
	emit Emitter
	log  *slog.Logger
	now  func() time.Time

	mu sync.Mutex
	qs map[string]*entry
}

// New builds a store over dir and loads what a previous guestd left there.
func New(dir string, emit Emitter, log *slog.Logger, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	s := &Store{dir: dir, emit: emit, log: log, now: now, qs: map[string]*entry{}}
	s.load()
	return s
}

func (s *Store) load() {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		var q Question
		if json.Unmarshal(b, &q) != nil || q.ID == "" {
			continue
		}
		en := &entry{q: q, done: make(chan struct{})}
		if !q.Open() {
			close(en.done)
		}
		s.qs[q.ID] = en
	}
}

// Clean normalises a text field: valid UTF-8, trimmed, capped at n bytes on
// a rune boundary.
func Clean(s string, n int) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "?")
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// Open starts a question and announces it.
func (s *Store) Open(agent, window, text string, options []string, timeout time.Duration) (Question, error) {
	text = Clean(text, TextCap)
	if text == "" {
		return Question{}, fmt.Errorf("%w: the question is empty", ErrInvalid)
	}
	var opts []string
	seen := map[string]bool{}
	for _, o := range options {
		o = Clean(o, OptionCap)
		if o == "" || seen[strings.ToLower(o)] {
			continue
		}
		seen[strings.ToLower(o)] = true
		opts = append(opts, o)
	}
	if len(opts) > MaxOptions {
		return Question{}, fmt.Errorf("%w: at most %d options", ErrInvalid, MaxOptions)
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaxTimeout {
		timeout = MaxTimeout
	}
	now := s.now().UTC()
	q := Question{
		ID: uuid.Must(uuid.NewV7()).String(), Agent: agent, Window: window, Text: text, Options: opts,
		TimeoutS: uint32(timeout / time.Second), Created: now, Expires: now.Add(timeout), State: StateOpen,
	}
	s.mu.Lock()
	open := 0
	for _, e := range s.qs {
		if e.q.Open() {
			open++
		}
	}
	if open >= MaxOpen {
		s.mu.Unlock()
		return Question{}, ErrTooMany
	}
	s.qs[q.ID] = &entry{q: q, done: make(chan struct{})}
	s.persist(q)
	s.mu.Unlock()
	s.emit(q)
	return q, nil
}

// Get returns a question.
func (s *Store) Get(id string) (Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.qs[id]
	if e == nil {
		return Question{}, ErrNotFound
	}
	return e.q, nil
}

// Wait returns the question once it is closed, or as it stands when ctx
// ends first.
func (s *Store) Wait(ctx context.Context, id string) (Question, error) {
	s.mu.Lock()
	e := s.qs[id]
	s.mu.Unlock()
	if e == nil {
		return Question{}, ErrNotFound
	}
	select {
	case <-e.done:
	case <-ctx.Done():
	}
	return s.Get(id)
}

// Answer closes a question from hostd's AnswerQuestion. A repeat for a
// question already closed is a no-op, so the api may resend.
func (s *Store) Answer(id, status, answer string) error {
	switch status {
	case StateAnswered, StateCancelled, StateExpired, StateNoChannel:
	default:
		return fmt.Errorf("%w: unknown status %q", ErrInvalid, status)
	}
	_, err := s.close(id, status, Clean(answer, AnswerCap))
	return err
}

// Cancel closes a question from the guest side (the asker gave up, or the
// timeout passed) and tells hostd.
func (s *Store) Cancel(id, state string) error {
	q, err := s.close(id, state, "")
	if err != nil {
		return err
	}
	if q != nil {
		s.emit(*q)
	}
	return nil
}

// close marks a question closed and returns it, or nil if it was already.
func (s *Store) close(id, state, answer string) (*Question, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.qs[id]
	if e == nil {
		return nil, ErrNotFound
	}
	if !e.q.Open() {
		return nil, nil
	}
	e.q.State = state
	if state == StateAnswered {
		e.q.Answer = answer
	}
	e.q.Closed = s.now().UTC()
	close(e.done)
	s.persist(e.q)
	q := e.q
	return &q, nil
}

// Pending lists the open questions, oldest first: what is re-announced on
// a new hostd connection.
func (s *Store) Pending() []Question {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Question
	for _, e := range s.qs {
		if e.q.Open() {
			out = append(out, e.q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out
}

// Sweep expires open questions past their deadline and forgets closed ones
// older than Keep.
func (s *Store) Sweep() {
	now := s.now()
	var expired []string
	s.mu.Lock()
	for id, e := range s.qs {
		switch {
		case e.q.Open() && !now.Before(e.q.Expires):
			expired = append(expired, id)
		case !e.q.Open() && now.Sub(e.q.Closed) > Keep:
			delete(s.qs, id)
			_ = os.Remove(s.file(id)) // a leftover file is reloaded as closed and swept again
		}
	}
	s.mu.Unlock()
	for _, id := range expired {
		_ = s.Cancel(id, StateExpired) // gone meanwhile is fine
	}
}

// Run sweeps every few seconds until ctx ends.
func (s *Store) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Sweep()
		}
	}
}

func (s *Store) file(id string) string { return filepath.Join(s.dir, id+".json") }

// persist writes the question file; called with mu held. A failure is
// logged by reason only and the question lives on in memory.
func (s *Store) persist(q Question) {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		s.log.Warn("could not create the questions directory", "event", "agent_question", "reason", "mkdir")
		return
	}
	b, _ := json.Marshal(q) // plain struct
	tmp := s.file(q.ID) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		s.log.Warn("could not write a question", "event", "agent_question", "reason", "write")
		return
	}
	if err := os.Rename(tmp, s.file(q.ID)); err != nil {
		s.log.Warn("could not write a question", "event", "agent_question", "reason", "rename")
	}
}
