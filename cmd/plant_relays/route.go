package main

import (
	"math"
	"slices"

	"github.com/gcapell/replicant/model"
)

// reach is how close (in light years) a star must be to another relay star
// to be part of the relay mesh.
const reach = 7.5

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

// connectable returns the candidates that can join the mesh: those within
// reach of a mesh star, or of a candidate that can. They come in the order
// the mesh would grow, nearest to it first, so any prefix of the result is
// itself connected to the mesh.
func connectable(mesh, candidates []star) []star {
	covered := slices.Clone(mesh)
	left := slices.Clone(candidates)
	var out []star
	for {
		best, bestD := -1, math.Inf(1)
		for i, c := range left {
			if _, d, ok := nearest(c.Pos, covered, ""); ok && d < reach && d < bestD {
				best, bestD = i, d
			}
		}
		if best < 0 {
			return out
		}
		out = append(out, left[best])
		covered = append(covered, left[best])
		left = slices.Delete(left, best, best+1)
	}
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
