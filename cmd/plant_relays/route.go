package main

import (
	"math"
	"slices"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// reach is how close (in light years) a star must be to another relay star
// to be part of the relay mesh.
const reach = auto.RelayReach

// star is a named point in space.
type star struct {
	Name       string
	Pos        model.Position
	EntryPoint string // "" if we don't know it yet
}

func starPos(s star) model.Position { return s.Pos }

// nearest returns the star in set nearest to p (other than one named skip),
// and its distance. ok is false if there is none.
func nearest(p model.Position, set []star, skip string) (s star, d float64, ok bool) {
	d = math.Inf(1)
	for _, c := range set {
		if c.Name == skip {
			continue
		}
		if cd := p.Distance(c.Pos); cd < d {
			s, d, ok = c, cd, true
		}
	}
	return s, d, ok
}

// connectable returns the candidates that can join the mesh, in the order
// the mesh would grow (auto.Connectable).
func connectable(mesh, candidates []star) []star {
	return auto.Connectable(mesh, candidates, starPos)
}

// without returns the stars in all that aren't in some.
func without(all, some []star) []star {
	return slices.DeleteFunc(slices.Clone(all), func(s star) bool {
		return slices.ContainsFunc(some, func(o star) bool { return o.Name == s.Name })
	})
}

// stop is one star on a planned route.
type stop struct {
	star
	Leg     float64 // distance from the previous stop (or start)
	Total   float64 // distance travelled so far, including any fetch
	Via     string  // nearest other relay star in the finished mesh
	ViaDist float64
}

// describe annotates route, which starts from start after travelling lead
// (to fetch relays). mesh is every relay star once the plan is done.
func describe(start model.Position, lead float64, route, mesh []star) []stop {
	stops := make([]stop, 0, len(route))
	at, total := start, lead
	for _, s := range route {
		leg := at.Distance(s.Pos)
		total += leg
		via, vd, _ := nearest(s.Pos, mesh, s.Name)
		stops = append(stops, stop{star: s, Leg: leg, Total: total, Via: via.Name, ViaDist: vd})
		at = s.Pos
	}
	return stops
}
