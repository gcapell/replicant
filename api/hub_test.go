package api

// Note: run with a dummy key, e.g. API_KEY=x go test ./api (api's init exits without one).

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/gcapell/replicant/model"
)

// fakeStream feeds events from a channel; it returns when something is sent on
// drop (simulating a dropped connection) or ctx is done. It records each lastID it's called with.
type fakeStream struct {
	events  chan Event
	drop    chan struct{}
	lastIDs chan string
}

func newFakeStream() *fakeStream {
	return &fakeStream{events: make(chan Event), drop: make(chan struct{}), lastIDs: make(chan string, 10)}
}

func (f *fakeStream) stream(ctx context.Context, lastID string, fn func(Envelope, any) error) error {
	f.lastIDs <- lastID
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.drop:
			return errors.New("stream dropped")
		case ev := <-f.events:
			if err := fn(ev.Env, ev.Payload); err != nil {
				return err
			}
		}
	}
}

func tempIDFile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "sub", "eventID.txt") // "sub" checks the dir gets created
}

func waiterCount(h *Hub) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.waiters)
}

func ev(id, device string, payload any) Event {
	return Event{Env: Envelope{ID: id, DeviceCode: device, CreatedAt: time.Now()}, Payload: payload}
}

func TestHubWakesMatchingWaiterOnly(t *testing.T) {
	idFile := tempIDFile(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFakeStream()
	h := newHub(f.stream, idFile)
	go h.run(ctx)

	wantA := h.Expect(ForDevice[model.DirectiveCompleted]("A"))
	wantB := h.Expect(ForDevice[model.DirectiveCompleted]("B"))

	f.events <- ev("1", "A", &model.DirectivePaused{}) // wrong type
	f.events <- ev("2", "B", &model.DirectiveCompleted{Directive: "gather_salvage"})

	got, err := wantB.Wait(ctx)
	if err != nil || got.Env.ID != "2" {
		t.Fatalf("B: got %+v, %v; want event 2", got, err)
	}

	short, done := context.WithTimeout(ctx, 50*time.Millisecond)
	defer done()
	if _, err := wantA.Wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("A: got err %v, want deadline exceeded", err)
	}
	if n := waiterCount(h); n != 0 {
		t.Errorf("%d waiters left registered, want 0", n)
	}
}

func TestHubEventBeforeWaitIsNotLost(t *testing.T) {
	idFile := tempIDFile(t)
	h := newHub(nil, idFile)
	w := h.Expect(AnyOf(
		ForDevice[model.DirectiveCompleted]("C"),
		ForDevice[model.DirectiveCleared]("C"),
	))
	h.dispatch(ev("9", "C", &model.DirectiveCleared{}))
	got, err := w.Wait(context.Background())
	if err != nil || got.Env.ID != "9" {
		t.Fatalf("got %+v, %v; want event 9", got, err)
	}
}

func TestHubIgnoresStaleEvents(t *testing.T) {
	idFile := tempIDFile(t)
	h := newHub(nil, idFile)
	w := h.Expect(ForDevice[model.DirectiveCompleted]("C"))
	old := ev("1", "C", &model.DirectiveCompleted{})
	old.Env.CreatedAt = time.Now().Add(-time.Hour) // e.g. replayed on resume
	h.dispatch(old)
	h.dispatch(ev("2", "C", &model.DirectiveCompleted{}))
	got, _ := w.Wait(context.Background())
	if got.Env.ID != "2" {
		t.Fatalf("got event %s, want 2 (event 1 is stale)", got.Env.ID)
	}
}

func TestHubPersistsAndResumesEventID(t *testing.T) {
	idFile := tempIDFile(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFakeStream()
	h := newHub(f.stream, idFile)
	go h.run(ctx)

	if id := <-f.lastIDs; id != "" {
		t.Fatalf("first connect lastID = %q, want empty", id)
	}
	w := h.Expect(ForDevice[model.DirectiveCompleted]("X"))
	f.events <- ev("100-0", "Y", nil) // undecodable payload still advances the ID
	f.events <- ev("101-0", "X", &model.DirectiveCompleted{})
	if _, err := w.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	// The ID is saved just after dispatch, so allow it a moment to land.
	deadline := time.Now().Add(time.Second)
	for loadEventID(idFile) != "101-0" {
		if time.Now().After(deadline) {
			t.Fatalf("saved ID = %q, want 101-0", loadEventID(idFile))
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Drop the connection: the hub should reconnect from the last ID.
	f.drop <- struct{}{}
	select {
	case id := <-f.lastIDs:
		if id != "101-0" {
			t.Fatalf("reconnect lastID = %q, want 101-0", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hub did not reconnect")
	}

	// A fresh hub starts from the saved file.
	cancel()
	f2 := newFakeStream()
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	go newHub(f2.stream, idFile).run(ctx2)
	if id := <-f2.lastIDs; id != "101-0" {
		t.Fatalf("new hub lastID = %q, want 101-0", id)
	}
}

func TestWaitReturnsErrorWhenHubStops(t *testing.T) {
	idFile := tempIDFile(t)
	ctx, cancel := context.WithCancel(context.Background())
	f := newFakeStream()
	h := newHub(f.stream, idFile)
	go h.run(ctx)
	w := h.Expect(ForDevice[model.DirectiveCompleted]("Z"))
	cancel()
	if _, err := w.Wait(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("got err %v, want context.Canceled", err)
	}
}
