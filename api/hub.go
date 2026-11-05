package api

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// EventIDFile holds the ID of the most recently processed SSE event, so a
// restarted Hub resumes the stream where the last one left off. NewHub reads it once.
var EventIDFile = "/tmp/replicant/eventID.txt"

// staleSlack is how far before a waiter registered an event may have been
// created and still match it, to tolerate clock skew between us and the server.
// Older events (e.g. replayed on resume) never wake a waiter.
const staleSlack = 30 * time.Second

// Event is one decoded SSE event. Payload is a pointer to the type registered
// for Env.Event in model.EventRegistry (e.g. *model.DirectiveCompleted), or nil
// if it could not be decoded.
type Event struct {
	Env     Envelope
	Payload any
}

// Hub owns a single SSE connection and wakes Waiters whose match function
// accepts an incoming event. Each Waiter fires at most once.
type Hub struct {
	stream func(ctx context.Context, lastID string, fn func(Envelope, any) error) error
	idFile string

	mu      sync.Mutex
	waiters map[*Waiter]struct{}
	err     error         // why the hub stopped; valid once done is closed
	done    chan struct{} // closed when the hub stops
}

// Waiter is a registered interest in one event. Create it with Hub.Expect
// *before* doing whatever will cause the event, then call Wait.
type Waiter struct {
	hub        *Hub
	match      func(Event) bool
	registered time.Time
	ch         chan Event // capacity 1, so dispatch never blocks
}

// NewHub starts reading the event stream, resuming from EventIDFile if present,
// and reconnecting (with backoff) whenever the stream drops. It runs until ctx is done.
func NewHub(ctx context.Context) *Hub {
	h := newHub(SSEStream, EventIDFile)
	go h.run(ctx)
	return h
}

func newHub(stream func(context.Context, string, func(Envelope, any) error) error, idFile string) *Hub {
	return &Hub{
		stream:  stream,
		idFile:  idFile,
		waiters: make(map[*Waiter]struct{}),
		done:    make(chan struct{}),
	}
}

func (h *Hub) run(ctx context.Context) {
	lastID := loadEventID(h.idFile)
	log.Printf("hub: resuming events after %q", lastID)
	backoff := time.Second
	for {
		err := h.stream(ctx, lastID, func(env Envelope, payload any) error {
			h.dispatch(Event{Env: env, Payload: payload})
			if env.ID != "" {
				lastID = env.ID
				if err := saveEventID(h.idFile, lastID); err != nil {
					log.Printf("hub: saving event ID: %v", err)
				}
			}
			backoff = time.Second // we're receiving events, so the connection is healthy
			return nil
		})
		if ctx.Err() != nil {
			h.stop(ctx.Err())
			return
		}
		log.Printf("hub: stream ended (%v); reconnecting in %s", err, backoff)
		select {
		case <-ctx.Done():
			h.stop(ctx.Err())
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

// dispatch delivers ev to every waiter that matches it, and removes those waiters.
func (h *Hub) dispatch(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for w := range h.waiters {
		if !ev.Env.CreatedAt.IsZero() && ev.Env.CreatedAt.Before(w.registered.Add(-staleSlack)) {
			continue
		}
		if w.match(ev) {
			w.ch <- ev // never blocks: capacity 1 and each waiter is sent to once
			delete(h.waiters, w)
		}
	}
}

func (h *Hub) stop(err error) {
	h.mu.Lock()
	h.err = err
	h.mu.Unlock()
	close(h.done)
}

// Expect registers a waiter for the first event (from now on) that match accepts.
func (h *Hub) Expect(match func(Event) bool) *Waiter {
	w := &Waiter{hub: h, match: match, registered: time.Now(), ch: make(chan Event, 1)}
	h.mu.Lock()
	h.waiters[w] = struct{}{}
	h.mu.Unlock()
	return w
}

// Wait blocks until the waiter's event arrives, ctx is done, or the hub stops.
// It may only be called once.
func (w *Waiter) Wait(ctx context.Context) (Event, error) {
	select {
	case ev := <-w.ch:
		return ev, nil
	case <-ctx.Done():
		w.Cancel()
		return Event{}, ctx.Err()
	case <-w.hub.done:
		select {
		case ev := <-w.ch: // delivered just before the hub stopped
			return ev, nil
		default:
		}
		w.Cancel()
		w.hub.mu.Lock()
		err := w.hub.err
		w.hub.mu.Unlock()
		return Event{}, errors.Join(errors.New("event hub stopped"), err)
	}
}

// Cancel unregisters the waiter. It is safe to call more than once, or after Wait.
func (w *Waiter) Cancel() {
	w.hub.mu.Lock()
	delete(w.hub.waiters, w)
	w.hub.mu.Unlock()
}

// ForDevice matches events with payload type T about the given device.
func ForDevice[T any](device string) func(Event) bool {
	return func(ev Event) bool {
		_, ok := ev.Payload.(*T)
		return ok && ev.Env.DeviceCode == device
	}
}

// AnyOf matches events accepted by any of ms.
func AnyOf(ms ...func(Event) bool) func(Event) bool {
	return func(ev Event) bool {
		for _, m := range ms {
			if m(ev) {
				return true
			}
		}
		return false
	}
}

func loadEventID(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("hub: reading event ID: %v", err)
		}
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveEventID writes atomically (temp file + rename) so a crash mid-write
// never leaves a truncated ID behind.
func saveEventID(path, id string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "eventID-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(id + "\n"); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
