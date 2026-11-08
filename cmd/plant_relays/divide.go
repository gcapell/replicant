package main

import (
	"slices"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// Phase 2: share the stars to plant among the vessels.

// team is a vessel with its fetch (if any) and its route.
type team struct {
	Vessel vessel
	Fetch  fetch
	Start  model.Position // where its route starts: the belt, or where it is
	Relays []string       // what it'll carry: stowed, then fetched
	Route  []star
}

func newTeam(v vessel, f fetch) team {
	t := team{Vessel: v, Fetch: f, Start: v.Pos, Relays: slices.Clone(v.Relays)}
	if f.Loc != "" {
		t.Start = f.Pos
	}
	for _, r := range f.Relays {
		t.Relays = append(t.Relays, r.Code)
	}
	return t
}

func teamStart(t *team) model.Position { return t.Start }

// cost is the team's whole trip: to its belt, then along its route.
func (t *team) cost() float64 {
	return t.Fetch.Dist + auto.RouteLength(t.Start, t.Route, starPos)
}

func (t *team) full() bool { return len(t.Route) >= len(t.Relays) }

// divide shares targets among the teams that hold relays, setting each
// team's Route, and returns any targets left over (when there aren't enough
// relays). It picks one spread-out seed star per team, gives each seed the
// nearest team, then repeatedly lets the team with the cheapest trip so far
// (that still has a relay to spare) take the unclaimed target nearest its
// stars. Each route visits its nearest star first, then follows a short path.
func divide(teams []team, targets []star) []star {
	var active []*team
	for i := range teams {
		if len(teams[i].Relays) > 0 {
			active = append(active, &teams[i])
		}
	}
	k := min(len(active), len(targets))
	if k == 0 {
		return targets
	}
	seeds := auto.Seeds(targets, k, starPos)
	groups := make([][]star, k)
	for g, s := range seeds {
		groups[g] = []star{targets[s]}
	}
	who := auto.Assign(active, teamStart, groups, starPos)
	members := make([]*team, k)
	for g, a := range who {
		members[g] = active[a]
		members[g].Route = groups[g]
	}

	left := without(targets, slices.Concat(groups...))
	for len(left) > 0 {
		var next *team
		for _, t := range members {
			if !t.full() && (next == nil || t.cost() < next.cost()) {
				next = t
			}
		}
		if next == nil {
			break
		}
		j := 0
		for i, s := range left {
			if auto.DistToSet(s.Pos, next.Route, starPos) < auto.DistToSet(left[j].Pos, next.Route, starPos) {
				j = i
			}
		}
		next.Route = auto.Order(next.Start, append(next.Route, left[j]), starPos)
		left = slices.Delete(left, j, j+1)
	}
	return left
}
