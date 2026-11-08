package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/gcapell/replicant/api"
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

// listEvents prints every event from the stream, starting after event since
// (or with new events, if since is ""), until ctx is done. If the stream
// drops it reconnects after the last event it saw, waiting 1s, doubling up to
// a minute, and back to 1s once events arrive.
func listEvents(ctx context.Context, stream streamFunc, w io.Writer, since string) error {
	p := &printer{w: w}
	lastID := since
	backoff := firstBackoff
	log.Printf("events: listening after %q", lastID)
	for {
		err := stream(ctx, lastID, func(env api.Envelope, payload any) error {
			p.print(env, payload)
			if env.ID != "" {
				lastID = env.ID
			}
			backoff = firstBackoff
			return nil
		})
		p.endLine()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("events: stream ended (%v); reconnecting after %q in %s", err, lastID, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

// printer writes one line per event, except that runs of boring events share
// a line.
type printer struct {
	w       io.Writer
	midLine bool // the last thing written was a boring event's name
}

func (p *printer) print(env api.Envelope, payload any) {
	if boring[env.Event] {
		fmt.Fprintf(p.w, "%s, ", env.Event)
		p.midLine = true
		return
	}
	p.endLine()
	fmt.Fprintln(p.w, describe(env, payload))
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
