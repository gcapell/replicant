package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
	"github.com/sanity-io/litter"
)

// boring events are printed as just their name, several to a line.
var boring = map[string]bool{
	"ami.mining.digest": true,
	"ami.survey.digest": true,
	"site.depleted":     true,
}

// firstBackoff is how long to wait before the first reconnection.
var firstBackoff = time.Second

// streamFunc is api.SSEStream, replaceable in tests.
type streamFunc func(ctx context.Context, id string, fn func(api.Envelope, any) error) error

// received is one event off the stream.
type received struct {
	env     api.Envelope
	payload any // nil if it wouldn't decode
}

// receive reads the stream, starting after event since (or with new events,
// if since is ""), and sends each event to out until ctx is done, then closes
// out. If the stream drops it reconnects after the last event it saw, waiting
// 1s, doubling up to a minute, and back to 1s once events arrive.
func receive(ctx context.Context, stream streamFunc, since string, out chan<- received) {
	defer close(out)
	lastID := since
	backoff := firstBackoff
	log.Printf("events: receiving after %q", lastID)
	for {
		err := stream(ctx, lastID, func(env api.Envelope, payload any) error {
			select {
			case out <- received{env, payload}:
			case <-ctx.Done():
				return ctx.Err()
			}
			if env.ID != "" {
				lastID = env.ID
			}
			backoff = firstBackoff
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		log.Printf("events: stream ended (%v); reconnecting after %q in %s", err, lastID, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

// recordAll records each event from in in the events table, applies it to
// the rest of the store if it's one we handle, and prints it, until in is
// closed. Recording and applying share a transaction, so the events table
// says exactly what has been applied. Any failure stops it.
//
// An event that's already recorded is an error, unless replaying (we were
// asked to start from an earlier event), when it's printed, marked
// "(already recorded)", and not applied again.
func recordAll(ctx context.Context, db *sql.DB, in <-chan received, p *printer, replaying bool) error {
	return execute(ctx, db, in, p, replaying, nil)
}

// execute is recordAll, also running tasks if ex isn't nil: after each new
// event is applied (and committed), the handlers of the tasks it concerns
// (tasksToWake), and every assigned task each time ex.sweep fires. Both
// happen on this goroutine, between events, so no task's handler runs twice
// at once. Replayed events (replaying) wake nothing. A failing handler stops
// it.
func execute(ctx context.Context, db *sql.DB, in <-chan received, p *printer, replaying bool, ex *executor) error {
	var sweep <-chan time.Time
	if ex != nil {
		sweep = ex.sweep
	}
	for {
		select {
		case r, ok := <-in:
			if !ok {
				return ctx.Err()
			}
			recorded, err := recordAndApply(ctx, db, r)
			if err != nil {
				return fmt.Errorf("%w\n%s", err, fullEvent(r.env))
			}
			if !recorded && !replaying {
				return fmt.Errorf("event %s (%s) is already recorded: the stream resent it", r.env.ID, r.env.Event)
			}
			p.print(r, !recorded)
			if ex == nil || !recorded || replaying {
				continue
			}
			ids, err := tasksToWake(ctx, db, r.env)
			if err != nil {
				return fmt.Errorf("%w\n%s", err, fullEvent(r.env))
			}
			if len(ids) == 0 {
				continue
			}
			p.endLine()
			log.Printf("event %s %s (%s) wakes tasks %v", r.env.ID, r.env.Event, r.env.DeviceCode, ids)
			if err := runTasks(ctx, db, ids); err != nil {
				return fmt.Errorf("%w\nwoken by %s", err, fullEvent(r.env))
			}
		case <-sweep:
			p.endLine()
			log.Print("sweep: filing, assigning and running tasks")
			if err := sweepTasks(ctx, db, p.w); err != nil {
				return fmt.Errorf("sweep: %w", err)
			}
		}
	}
}

// executor is what execute needs to run tasks.
type executor struct {
	sweep <-chan time.Time // when to run every assigned task; nil for never
}

// recordAndApply records r and applies it, if it's an event we handle and
// it isn't already recorded. The handler fetches whatever it needs first, so
// the transaction is open only while writing.
func recordAndApply(ctx context.Context, db *sql.DB, r received) (recorded bool, err error) {
	e := r.env
	already, err := store.EventRecorded(ctx, db, e.ID)
	if err != nil || already {
		return false, err
	}
	var apply change
	if h, ok := handlers[e.Event]; ok {
		if r.payload == nil {
			return false, fmt.Errorf("payload wouldn't decode: %s", e.Payload)
		}
		if apply, err = h(ctx, db, e, r.payload); err != nil {
			return false, err
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	_, recorded, err = store.RecordEvent(ctx, tx, store.Event{
		ID: e.ID, Version: e.Version, Category: e.Category, Name: e.Event, CreatedAt: e.CreatedAt,
		DeviceCode: e.DeviceCode, DeviceType: e.DeviceType, ReplicantCode: e.ReplicantCode,
		Star: e.Star, Location: e.Location, Payload: e.Payload,
	})
	if err != nil || !recorded {
		return false, err
	}
	if apply != nil {
		if err := apply(ctx, tx); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// catchUp applies every recorded event called any of names again, oldest
// first across all of them (so a relay's device.deployed comes before its
// relay.activated), as if it had just arrived (but without recording it
// again). Each is applied in its own transaction; the first failure stops it.
// A name monitor doesn't handle is an error, before anything is applied.
func catchUp(ctx context.Context, db *sql.DB, names []string, p *printer) error {
	if len(names) == 0 {
		return fmt.Errorf("catch up: no event types given")
	}
	for _, name := range names {
		if _, ok := handlers[name]; !ok {
			return fmt.Errorf("catch up: monitor doesn't handle %q events", name)
		}
		if _, ok := model.EventRegistry[name]; !ok {
			return fmt.Errorf("catch up: no payload type registered for %q", name)
		}
	}
	events, err := store.EventsNamed(ctx, db, names...)
	if err != nil {
		return err
	}
	log.Printf("catch up: %d events (%s)", len(events), strings.Join(names, ", "))
	for _, e := range events {
		payload, err := model.EventRegistry[e.Name](e.Payload)
		if err != nil {
			return fmt.Errorf("catch up: payload wouldn't decode: %w\n%s", err, fullEvent(api.Envelope{ID: e.ID, Event: e.Name, Payload: e.Payload}))
		}
		env := api.Envelope{
			Version: e.Version, ID: e.ID, Category: e.Category, Event: e.Name, CreatedAt: e.CreatedAt,
			DeviceCode: e.DeviceCode, DeviceType: e.DeviceType, ReplicantCode: e.ReplicantCode,
			Star: e.Star, Location: e.Location, Payload: e.Payload,
		}
		apply, err := handlers[e.Name](ctx, db, env, payload)
		if err != nil {
			return fmt.Errorf("catch up: %w\n%s", err, fullEvent(env))
		}
		if apply != nil {
			if err := inTx(ctx, db, func(tx *sql.Tx) error { return apply(ctx, tx) }); err != nil {
				return fmt.Errorf("catch up: %w\n%s", err, fullEvent(env))
			}
		}
		p.print(received{env, payload}, false)
	}
	return nil
}

// printer writes one line per event, except that runs of boring events share
// a line.
type printer struct {
	w       io.Writer
	midLine bool // the last thing written was a boring event's name
}

func (p *printer) print(r received, already bool) {
	if boring[r.env.Event] && !already {
		fmt.Fprintf(p.w, "%s, ", r.env.Event)
		p.midLine = true
		return
	}
	p.endLine()
	line := describe(r.env, r.payload)
	if already {
		line = "(already recorded) " + line
	}
	fmt.Fprintln(p.w, line)
}

func (p *printer) endLine() {
	if p.midLine {
		fmt.Fprintln(p.w)
		p.midLine = false
	}
}

// describe is one event as a line (or more, for a big payload). An event we
// couldn't decode shows its raw JSON, marked UNDECODED.
func describe(env api.Envelope, payload any) string {
	head := fmt.Sprintf("%s %s %s", env.CreatedAt.Local().Format("15:04:05"), env.ID, env.Event)
	if env.DeviceCode != "" {
		head += " " + env.DeviceCode
	}
	if env.DeviceType != "" {
		head += " (" + env.DeviceType + ")"
	}
	if env.Location != "" {
		head += " @" + env.Location
	}
	if payload == nil {
		return head + " UNDECODED " + string(env.Payload)
	}
	return head + " " + litter.Sdump(payload)
}

// fullEvent describes an event in full, envelope and raw payload, for error
// messages.
func fullEvent(e api.Envelope) string {
	return fmt.Sprintf("event %s %s, created %s\n  device %s (%s), replicant %s, star %s, location %s, version %d, category %s\n  payload %s",
		e.ID, e.Event, e.CreatedAt.Format(time.RFC3339),
		e.DeviceCode, e.DeviceType, e.ReplicantCode, e.Star, e.Location, e.Version, e.Category, e.Payload)
}
