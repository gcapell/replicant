package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// Completing events: an idle heaven_vessel carrying a replicant_matrix goes
// to each ready event (everything there, including a beacon) and completes
// it.

// vessel is an idle heaven_vessel with a replicant aboard.
type vessel struct {
	Code string
	At   place
}

// completion is a ready event to complete.
type completion struct {
	Event string
	At    place
}

func vesselPos(v vessel) model.Position         { return v.At.Pos }
func completionPos(c completion) model.Position { return c.At.Pos }

type vesselRoute struct {
	Vessel vessel
	Stops  []completion
}

// readyEvents lists the needs we can complete now: everything the chosen
// option asks for is there, and so is a beacon.
func readyEvents(needs []need) []completion {
	var cs []completion
	for _, n := range sortedNeeds(needs) {
		if n.Met && n.Beacon {
			cs = append(cs, completion{Event: n.Event, At: place{Loc: n.Loc, Pos: n.Pos}})
		}
	}
	return cs
}

// planCompletions splits the events among the vessels: k = min(vessels,
// events) spread-out seeds, every other event joins its nearest seed, each
// group gets a different vessel (minimising the total distance to the
// groups), and each vessel visits its group's nearest event first, then
// follows a short path. It returns one route per vessel, in order.
func planCompletions(vessels []vessel, events []completion) []vesselRoute {
	routes := make([]vesselRoute, len(vessels))
	for i, v := range vessels {
		routes[i].Vessel = v
	}
	k := min(len(vessels), len(events))
	if k == 0 {
		return routes
	}
	groups := auto.GroupNearest(events, auto.Seeds(events, k, completionPos), completionPos)
	for g, vi := range auto.Assign(vessels, vesselPos, groups, completionPos) {
		routes[vi].Stops = auto.Order(vessels[vi].At.Pos, groups[g], completionPos)
	}
	return routes
}

func describeCompletions(routes []vesselRoute, events []completion) []string {
	if len(events) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("Completions: %d ready events", len(events))}
	if len(routes) == 0 {
		return append(lines, "    no idle heaven_vessel with a replicant_matrix")
	}
	for _, r := range routes {
		if len(r.Stops) == 0 {
			continue
		}
		var legs []string
		at := r.Vessel.At.Pos
		for _, s := range r.Stops {
			legs = append(legs, fmt.Sprintf("%s (%.1f ly)", s.At.Loc, at.Distance(s.At.Pos)))
			at = s.At.Pos
		}
		lines = append(lines, fmt.Sprintf("%s from %s: %s", r.Vessel.Code, r.Vessel.At.Loc, strings.Join(legs, " → ")))
	}
	return lines
}

// idleVessels returns the idle heaven_vessels carrying a replicant_matrix.
func idleVessels(ctx context.Context) ([]vessel, error) {
	ds, err := auto.Devices(ctx, "heaven_vessel")
	if err != nil {
		return nil, err
	}
	var vs []vessel
	for _, d := range ds {
		if auto.ReadyReplicant(d) {
			vs = append(vs, vessel{Code: d.DeviceCode, At: place{Loc: d.Location}})
		}
	}
	log.Printf("%d idle heaven_vessels with a replicant (of %d)", len(vs), len(ds))
	return vs, nil
}

// completeAll runs every vessel's route at once. A vessel whose travel
// fails stops; a completion that fails is logged and the vessel moves on.
func completeAll(ctx context.Context, hub *api.Hub, routes []vesselRoute) error {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, r := range routes {
		if len(r.Stops) == 0 {
			continue
		}
		wg.Go(func() {
			v := r.Vessel.Code
			for _, s := range r.Stops {
				if err := auto.GoTo(ctx, hub, v, s.At.Loc); err != nil {
					log.Printf("%s: FAILED: %v", v, err)
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", v, err))
					mu.Unlock()
					return
				}
				res, err := api.CompleteEvent(ctx, s.At.Loc, s.Event)
				if err != nil {
					log.Printf("%s: warning: completing %s failed: %v", v, s.Event, err)
					mu.Lock()
					errs = append(errs, fmt.Errorf("%s: %s: %w", v, s.Event, err))
					mu.Unlock()
					continue
				}
				log.Printf("%s: completed %s (%s): status %s", v, s.Event, res.Title, res.Status)
			}
			log.Printf("%s: route done", v)
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}
