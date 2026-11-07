package main

import (
	"math"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// system is a star system with an active relay that still has unscanned
// planets or moons.
type system struct {
	Name       string
	Pos        model.Position
	EntryPoint string // where the transport waits during the survey
	Planets    int    // unscanned planets
	Moons      int    // unscanned moons
	Unscanned  int    // Planets + Moons
}

func sysPos(s system) model.Position { return s.Pos }
func teamPos(t team) model.Position  { return t.Pos }

// team is a validated survey team.
type team struct {
	Transport  string
	Controller string
	Drones     []string
	Star       string // where the transport is now
	Pos        model.Position
}

// route is the plan for one team. Stops is empty for a team with nothing to do.
type route struct {
	Team  team
	Stops []system
}

// plan divides systems among teams and orders each team's share into a route.
// It returns one route per team, in the order of teams.
func plan(teams []team, systems []system) []route {
	k := min(len(teams), len(systems))
	routes := make([]route, len(teams))
	for i, t := range teams {
		routes[i].Team = t
	}
	if k == 0 {
		return routes
	}
	groups := group(systems, auto.Seeds(systems, k, sysPos))
	for g, ti := range auto.Assign(teams, teamPos, groups, sysPos) {
		t := teams[ti]
		routes[ti].Stops = auto.Order(t.Pos, groups[g], sysPos)
	}
	return routes
}

// group grows one group from each seed, keeping the work (unscanned planets
// and moons) balanced: the group with the least work so far takes the
// unclaimed system nearest to any of its members, until none are left.
func group(systems []system, seeds []int) [][]system {
	groups := make([][]system, len(seeds))
	load := make([]int, len(seeds))
	claimed := make([]bool, len(systems))
	for g, i := range seeds {
		groups[g] = []system{systems[i]}
		load[g] = systems[i].Unscanned
		claimed[i] = true
	}
	for left := len(systems) - len(seeds); left > 0; left-- {
		g := 0
		for h := range load {
			if load[h] < load[g] {
				g = h
			}
		}
		best, bestD := -1, math.Inf(1)
		for i, s := range systems {
			if claimed[i] {
				continue
			}
			if d := auto.DistToSet(s.Pos, groups[g], sysPos); d < bestD {
				best, bestD = i, d
			}
		}
		groups[g] = append(groups[g], systems[best])
		load[g] += systems[best].Unscanned
		claimed[best] = true
	}
	return groups
}
