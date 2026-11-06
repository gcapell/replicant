package main

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// minBeltStock is how much a belt must already hold to be a delivery target.
const minBeltStock = 1000

// claim reserves resources at one location for one freighter.
type claim struct {
	loc     string
	amounts map[model.ResourceType]int
}

// ledger hands out claims on stockpiles so freighters don't race for the same
// resources. Each decision re-reads the inventory and subtracts outstanding
// claims, so it also sees resources that appear (or vanish) for other reasons.
type ledger struct {
	inventory func(context.Context) ([]model.LocationInventory, error)
	stars     map[string]model.Position

	mu     sync.Mutex
	claims map[string]claim // by freighter; at most one each
}

func newLedger(inventory func(context.Context) ([]model.LocationInventory, error), stars []model.Star) *ledger {
	l := &ledger{inventory: inventory, stars: map[string]model.Position{}, claims: map[string]claim{}}
	for _, s := range stars {
		l.stars[s.Designation] = s.Position
	}
	return l
}

// Claim reserves up to free units at the nearest planet or moon (to from) that
// has unclaimed resources, replacing any claim freighter already held. With
// sameStar, only sources in from's star are considered. It returns "" if
// there's nothing to claim.
func (l *ledger) Claim(ctx context.Context, freighter, from string, free int, sameStar bool) (claim, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.claims, freighter)
	inv, err := l.inventory(ctx)
	if err != nil {
		return claim{}, err
	}
	c := l.choose(inv, from, free, sameStar)
	if c.loc != "" {
		l.claims[freighter] = c
	}
	return c, nil
}

// Release drops freighter's claim (once it has collected, or given up).
func (l *ledger) Release(freighter string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.claims, freighter)
}

// NearestBelt returns the nearest belt (to from) already holding more than
// minBeltStock resources, or "" if there are none.
func (l *ledger) NearestBelt(ctx context.Context, from string) (string, error) {
	inv, err := l.inventory(ctx)
	if err != nil {
		return "", err
	}
	return l.nearestBelt(inv, from), nil
}

func (l *ledger) nearestBelt(inv []model.LocationInventory, from string) string {
	best, bestD := "", math.Inf(1)
	for _, li := range inv {
		if !strings.Contains(li.Location, "-BELT-") || li.Total() <= minBeltStock {
			continue
		}
		d := l.distance(from, li.Location)
		if d < bestD || (d == bestD && li.Location < best) {
			best, bestD = li.Location, d
		}
	}
	return best
}

// choose picks the nearest source with unclaimed resources (largest first
// within a star) and claims up to free units of it. Callers hold l.mu.
func (l *ledger) choose(inv []model.LocationInventory, from string, free int, sameStar bool) claim {
	if free <= 0 {
		return claim{}
	}
	type source struct {
		loc   string
		avail map[model.ResourceType]int
		total int
		dist  float64
	}
	var sources []source
	for _, li := range inv {
		star := auto.StarOf(li.Location)
		if !auto.IsPlanetOrMoon(star, li.Location) || (sameStar && star != auto.StarOf(from)) {
			continue
		}
		d := l.distance(from, li.Location)
		if math.IsInf(d, 1) {
			continue
		}
		avail := li.Amounts()
		for _, c := range l.claims {
			if c.loc == li.Location {
				for t, q := range c.amounts {
					avail[t] -= q
				}
			}
		}
		total := 0
		for _, q := range avail {
			total += max(q, 0)
		}
		if total > 0 {
			sources = append(sources, source{li.Location, avail, total, d})
		}
	}
	if len(sources) == 0 {
		return claim{}
	}
	s := slices.MinFunc(sources, func(a, b source) int {
		return cmp.Or(cmp.Compare(a.dist, b.dist), cmp.Compare(b.total, a.total), strings.Compare(a.loc, b.loc))
	})
	return claim{loc: s.loc, amounts: takeUpTo(s.avail, free)}
}

// distance between two locations: straight-line between their stars, 0 within
// a star, +Inf if either star's position is unknown.
func (l *ledger) distance(a, b string) float64 {
	sa, sb := auto.StarOf(a), auto.StarOf(b)
	if sa == sb {
		return 0
	}
	pa, ok1 := l.stars[sa]
	pb, ok2 := l.stars[sb]
	if !ok1 || !ok2 {
		return math.Inf(1)
	}
	d := pa.Delta(pb)
	return math.Sqrt(d.X*d.X + d.Y*d.Y + d.Z*d.Z)
}

// takeUpTo chooses up to free units from avail, largest resources first.
func takeUpTo(avail map[model.ResourceType]int, free int) map[model.ResourceType]int {
	types := make([]model.ResourceType, 0, len(avail))
	for t, q := range avail {
		if q > 0 {
			types = append(types, t)
		}
	}
	slices.SortFunc(types, func(a, b model.ResourceType) int {
		return cmp.Or(cmp.Compare(avail[b], avail[a]), strings.Compare(string(a), string(b)))
	})
	take := map[model.ResourceType]int{}
	for _, t := range types {
		if free <= 0 {
			break
		}
		q := min(avail[t], free)
		take[t] = q
		free -= q
	}
	return take
}
