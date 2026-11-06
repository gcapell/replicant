package main

import (
	"math"
	"slices"

	"github.com/gcapell/replicant/model"
)

// reach is how close (in light years) a star must be to a star with an
// ftl_relay for us to plant there.
const reach = 7.5

// star is a named point in space.
type star struct {
	Name       string
	Pos        model.Position
	Explored   bool
	EntryPoint string // "" if we don't know it yet
}

// stop is one star on the planned route.
type stop struct {
	star
	Leg     float64 // distance from the previous stop (or start)
	Total   float64 // distance travelled so far
	Via     string  // relay star (existing or planted earlier) that puts it in reach
	ViaDist float64
}

func dist(a, b model.Position) float64 {
	return math.Sqrt((a.X-b.X)*(a.X-b.X) + (a.Y-b.Y)*(a.Y-b.Y) + (a.Z-b.Z)*(a.Z-b.Z))
}

// nearest returns the star in covered nearest to p, and its distance.
// ok is false if covered is empty.
func nearest(p model.Position, covered []star) (s star, d float64, ok bool) {
	d = math.Inf(1)
	for _, c := range covered {
		if cd := dist(p, c.Pos); cd < d {
			s, d, ok = c, cd, true
		}
	}
	return s, d, ok
}

// valid reports whether every star in route is within reach of a relay star:
// one of relays, or a stop earlier in the route.
func valid(route, relays []star) bool {
	covered := slices.Clone(relays)
	for _, s := range route {
		if _, d, ok := nearest(s.Pos, covered); !ok || d >= reach {
			return false
		}
		covered = append(covered, s)
	}
	return true
}

func length(start model.Position, route []star) float64 {
	total, at := 0.0, start
	for _, s := range route {
		total += dist(at, s.Pos)
		at = s.Pos
	}
	return total
}

// plan chooses up to limit stars from candidates to plant relays at, starting
// from start, and orders them to keep the total distance short. Each stop must
// be within reach of one of relays or an earlier stop. It picks the nearest
// reachable candidate each time, then improves the order with 2-opt moves that
// keep the route valid.
func plan(start model.Position, relays, candidates []star, limit int) []star {
	covered := slices.Clone(relays)
	left := slices.Clone(candidates)
	var route []star
	at := start
	for len(route) < limit {
		best, bestD := -1, math.Inf(1)
		for i, c := range left {
			if _, d, ok := nearest(c.Pos, covered); !ok || d >= reach {
				continue
			}
			if d := dist(at, c.Pos); d < bestD {
				best, bestD = i, d
			}
		}
		if best < 0 {
			break
		}
		s := left[best]
		left = slices.Delete(left, best, best+1)
		route = append(route, s)
		covered = append(covered, s)
		at = s.Pos
	}
	return twoOpt(start, route, relays)
}

// planInTwo plans the best route for the carrying relays the vessel holds,
// then continues from its end to every other reachable candidate, so the
// first part isn't distorted by stops it can't make yet.
func planInTwo(start model.Position, relays, candidates []star, carrying int) []star {
	first := plan(start, relays, candidates, carrying)
	if len(first) == 0 {
		return nil
	}
	left := slices.DeleteFunc(slices.Clone(candidates), func(c star) bool {
		return slices.ContainsFunc(first, func(s star) bool { return s.Name == c.Name })
	})
	covered := append(slices.Clone(relays), first...)
	rest := plan(first[len(first)-1].Pos, covered, left, len(left))
	return append(first, rest...)
}

// twoOpt repeatedly reverses a section of route when that makes it shorter
// and still valid, until no reversal helps.
func twoOpt(start model.Position, route, relays []star) []star {
	const eps = 1e-9
	best := length(start, route)
	for improved := true; improved; {
		improved = false
		for i := range len(route) - 1 {
			for j := i + 1; j < len(route); j++ {
				slices.Reverse(route[i : j+1])
				if l := length(start, route); l < best-eps && valid(route, relays) {
					best, improved = l, true
				} else {
					slices.Reverse(route[i : j+1])
				}
			}
		}
	}
	return route
}

// describe annotates route with distances and the relay star that makes each
// stop reachable.
func describe(start model.Position, route, relays []star) []stop {
	covered := slices.Clone(relays)
	stops := make([]stop, 0, len(route))
	at, total := start, 0.0
	for _, s := range route {
		leg := dist(at, s.Pos)
		total += leg
		via, vd, _ := nearest(s.Pos, covered)
		stops = append(stops, stop{star: s, Leg: leg, Total: total, Via: via.Name, ViaDist: vd})
		covered = append(covered, s)
		at = s.Pos
	}
	return stops
}
