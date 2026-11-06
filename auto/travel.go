package auto

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

const (
	// TravelTimeout bounds a single trip; there's no ETA up front, so be generous.
	TravelTimeout = 12 * time.Hour
	// departedTimeout bounds the wait for the travel.departed event after a travel command.
	departedTimeout = 10 * time.Second
)

// GoTo sends device to loc and waits for it to arrive. It's a no-op if the
// device is already there, and just waits if it's already on its way there.
func GoTo(ctx context.Context, hub *api.Hub, device, loc string) error {
	d, err := api.Device(ctx, device)
	if err != nil {
		return err
	}
	if d.Status == "travelling" && d.Travel.FinalDestination == loc {
		return awaitTrip(ctx, hub, d)
	}
	if d.Location == loc && d.Status != "travelling" {
		return nil
	}
	// Register both before travel, so quick events aren't missed.
	departed := hub.Expect(api.ForDevice[model.TravelDeparted](device))
	w := expectArrival(hub, device)
	if err := api.Travel(ctx, device, loc); err != nil {
		departed.Cancel()
		w.Cancel()
		return fmt.Errorf("travel %s to %s: %w", device, loc, err)
	}
	log.Printf("%s travelling %s -> %s (ETA %s)", device, d.Location, loc, departureETA(ctx, departed, device))
	return waitArrival(ctx, w, device, loc)
}

// GoToAll sends all devices to loc concurrently and waits for them all to arrive.
func GoToAll(ctx context.Context, hub *api.Hub, devices []string, loc string) error {
	var wg sync.WaitGroup
	errs := make([]error, len(devices))
	for i, d := range devices {
		wg.Go(func() { errs[i] = GoTo(ctx, hub, d, loc) })
	}
	wg.Wait()
	return errors.Join(errs...)
}

// FinishTrip waits for device to finish its current trip, if it's on one.
func FinishTrip(ctx context.Context, hub *api.Hub, device string) error {
	d, err := api.Device(ctx, device)
	if err != nil {
		return err
	}
	if d.Status != "travelling" {
		return nil
	}
	return awaitTrip(ctx, hub, d)
}

// awaitTrip waits for d (fetched while travelling) to reach its final destination.
func awaitTrip(ctx context.Context, hub *api.Hub, d *model.Device) error {
	loc := d.Travel.FinalDestination
	log.Printf("%s already travelling to %s (ETA %s); waiting for arrival",
		d.DeviceCode, loc, FormatSeconds(d.Travel.RouteEtaSeconds))
	w := expectArrival(hub, d.DeviceCode)
	// Re-check, in case it arrived between the fetch and registering the waiter.
	d, err := api.Device(ctx, d.DeviceCode)
	if err != nil {
		w.Cancel()
		return err
	}
	if d.Status != "travelling" {
		w.Cancel()
		return nil
	}
	return waitArrival(ctx, w, d.DeviceCode, loc)
}

// expectArrival waits for device's next travel.arrived or travel.cancelled.
// We only get travel.arrived at the end of a trip (not for intermediate legs),
// so any arrival will do. Its destination isn't always the location we asked
// for: a surge to another system reports the bare star, with the real place in
// the event's Location.
func expectArrival(hub *api.Hub, device string) *api.Waiter {
	return hub.Expect(api.AnyOf(
		api.ForDevice[model.TravelArrived](device),
		api.ForDevice[model.TravelCancelled](device),
	))
}

// waitArrival waits (using w, from expectArrival) for device's trip to loc to end.
func waitArrival(ctx context.Context, w *api.Waiter, device, loc string) error {
	ctx, cancel := context.WithTimeout(ctx, TravelTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		return fmt.Errorf("waiting for %s to arrive at %s: %w", device, loc, err)
	}
	if c, ok := ev.Payload.(*model.TravelCancelled); ok {
		return fmt.Errorf("%s travel %s -> %s cancelled", device, c.Origin, c.Destination)
	}
	if ev.Env.Location != loc {
		log.Printf("warning: %s arrived at %s, not %s", device, ev.Env.Location, loc)
	} else {
		log.Printf("%s arrived at %s", device, loc)
	}
	return nil
}

// departureETA waits briefly for the travel.departed event and returns the trip
// time it reports, or "unknown" if it doesn't arrive in time.
func departureETA(ctx context.Context, w *api.Waiter, device string) string {
	ctx, cancel := context.WithTimeout(ctx, departedTimeout)
	defer cancel()
	ev, err := w.Wait(ctx)
	if err != nil {
		log.Printf("warning: no travel.departed event for %s: %v", device, err)
		return "unknown"
	}
	return FormatSeconds(ev.Payload.(*model.TravelDeparted).TravelTimeSeconds)
}

// FormatSeconds renders a duration in seconds like "3m22s".
func FormatSeconds(s float64) string {
	return time.Duration(s * float64(time.Second)).Round(time.Second).String()
}
