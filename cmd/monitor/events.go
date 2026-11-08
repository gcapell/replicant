package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/gcapell/replicant/api"
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

// recordAll records each event from in in the events table, and prints it,
// until in is closed. An event that's already recorded is an error, unless
// replaying (we were asked to start from an earlier event), when it's printed
// and marked "(already recorded)".
func recordAll(ctx context.Context, db store.Queryer, in <-chan received, p *printer, replaying bool) error {
	for r := range in {
		e := r.env
		_, recorded, err := store.RecordEvent(ctx, db, store.Event{
			ID: e.ID, Version: e.Version, Category: e.Category, Name: e.Event, CreatedAt: e.CreatedAt,
			DeviceCode: e.DeviceCode, DeviceType: e.DeviceType, ReplicantCode: e.ReplicantCode,
			Star: e.Star, Location: e.Location, Payload: e.Payload,
		})
		if err != nil {
			return err
		}
		if !recorded && !replaying {
			return fmt.Errorf("event %s (%s) is already recorded: the stream resent it", e.ID, e.Event)
		}
		p.print(r, !recorded)
	}
	return ctx.Err()
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
