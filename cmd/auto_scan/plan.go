package main

import (
	"math"
	"slices"

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

// maxCombos is the most seed sets we try exhaustively before falling back to
// picking seeds greedily.
const maxCombos = 200_000

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
	groups := group(systems, seeds(systems, k))
	for g, ti := range assign(teams, groups) {
		t := teams[ti]
		routes[ti].Stops = order(t.Pos, groups[g])
	}
	return routes
}

// seeds picks k systems (by index) that are as spread out as possible: it
// maximises the smallest distance between any two of them, breaking ties by
// the total distance between them.
func seeds(systems []system, k int) []int {
	if k == 1 {
		return []int{0}
	}
	if combos(len(systems), k) > maxCombos {
		return farthestFirst(systems, k)
	}
	var best []int
	bestMin, bestSum := -1.0, -1.0
	cur := make([]int, 0, k)
	var rec func(from int)
	rec = func(from int) {
		if len(cur) == k {
			mn, sum := spread(systems, cur)
			if mn > bestMin+1e-9 || (mn > bestMin-1e-9 && sum > bestSum+1e-9) {
				best, bestMin, bestSum = slices.Clone(cur), mn, sum
			}
			return
		}
		for i := from; i <= len(systems)-(k-len(cur)); i++ {
			cur = append(cur, i)
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)
	return best
}

// spread returns the smallest and the total pairwise distance between the
// chosen systems.
func spread(systems []system, chosen []int) (mn, sum float64) {
	mn = math.Inf(1)
	for i, a := range chosen {
		for _, b := range chosen[i+1:] {
			d := systems[a].Pos.Distance(systems[b].Pos)
			mn, sum = min(mn, d), sum+d
		}
	}
	return mn, sum
}

func combos(n, k int) float64 {
	c := 1.0
	for i := range k {
		c = c * float64(n-i) / float64(i+1)
	}
	return c
}

// farthestFirst starts from the two systems farthest apart, then repeatedly
// adds the system farthest from those already chosen.
func farthestFirst(systems []system, k int) []int {
	a, b, far := 0, 0, -1.0
	for i := range systems {
		for j := i + 1; j < len(systems); j++ {
			if d := systems[i].Pos.Distance(systems[j].Pos); d > far {
				a, b, far = i, j, d
			}
		}
	}
	chosen := []int{a, b}
	for len(chosen) < k {
		next, nextD := -1, -1.0
		for i, s := range systems {
			if slices.Contains(chosen, i) {
				continue
			}
			d := math.Inf(1)
			for _, c := range chosen {
				d = min(d, s.Pos.Distance(systems[c].Pos))
			}
			if d > nextD {
				next, nextD = i, d
			}
		}
		chosen = append(chosen, next)
	}
	return chosen
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
			if d := distToSet(s.Pos, groups[g]); d < bestD {
				best, bestD = i, d
			}
		}
		groups[g] = append(groups[g], systems[best])
		load[g] += systems[best].Unscanned
		claimed[best] = true
	}
	return groups
}

func distToSet(p model.Position, set []system) float64 {
	d := math.Inf(1)
	for _, s := range set {
		d = min(d, p.Distance(s.Pos))
	}
	return d
}

// assign gives each group a different team, minimising the total distance from
// each team to the nearest system in its group. It returns, for each group,
// the index of its team. There must be at least as many teams as groups.
func assign(teams []team, groups [][]system) []int {
	var best []int
	bestCost := math.Inf(1)
	cur := make([]int, 0, len(groups))
	used := make([]bool, len(teams))
	var rec func(cost float64)
	rec = func(cost float64) {
		if cost >= bestCost-1e-9 {
			return
		}
		g := len(cur)
		if g == len(groups) {
			best, bestCost = slices.Clone(cur), cost
			return
		}
		for t := range teams {
			if used[t] {
				continue
			}
			used[t] = true
			cur = append(cur, t)
			rec(cost + distToSet(teams[t].Pos, groups[g]))
			cur = cur[:len(cur)-1]
			used[t] = false
		}
	}
	rec(0)
	return best
}

// order visits the group's nearest system first, then the rest by a short
// path from there.
func order(start model.Position, group []system) []system {
	first := 0
	for i, s := range group {
		if start.Distance(s.Pos) < start.Distance(group[first].Pos) {
			first = i
		}
	}
	rest := slices.Delete(slices.Clone(group), first, first+1)
	return append([]system{group[first]}, auto.ShortRoute(group[first].Pos, rest, sysPos)...)
}
