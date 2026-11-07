package main

import (
	"cmp"
	"slices"

	"github.com/gcapell/replicant/model"
)

// freighter is an idle cargo_freighter and where it is.
type freighter struct {
	Code string
	Star string
	Pos  model.Position
}

// dispatch is the freighter sent to one assignment's source star.
type dispatch struct {
	Freighter freighter
	Dist      float64 // freighter's star to the source star
}

// assignFreighters gives each sourced assignment (Star != "") at most one
// freighter, at most one assignment per freighter. It covers as many
// assignments as it can, and among those matchings minimises the largest
// freighter-to-source distance. Within that limit each assignment, in plan
// order, tries its nearest freighters first. The result is keyed by index
// into as.
func assignFreighters(as []assignment, fs []freighter, starPos map[string]model.Position) map[int]dispatch {
	type edge struct {
		f int
		d float64
	}
	edges := map[int][]edge{} // assignment index -> freighters, nearest first
	var dists []float64
	var jobs []int
	for i, a := range as {
		if a.Star == "" {
			continue
		}
		jobs = append(jobs, i)
		p := starPos[a.Star]
		for j, f := range fs {
			d := f.Pos.Distance(p)
			edges[i] = append(edges[i], edge{j, d})
			dists = append(dists, d)
		}
		slices.SortStableFunc(edges[i], func(x, y edge) int { return cmp.Compare(x.d, y.d) })
	}
	if len(jobs) == 0 || len(fs) == 0 {
		return map[int]dispatch{}
	}

	// match finds a maximum matching using only edges no longer than limit
	// (Kuhn's augmenting paths), returning freighter index -> assignment.
	match := func(limit float64) map[int]int {
		owner := map[int]int{}
		var try func(i int, seen map[int]bool) bool
		try = func(i int, seen map[int]bool) bool {
			for _, e := range edges[i] {
				if e.d > limit || seen[e.f] {
					continue
				}
				seen[e.f] = true
				if o, taken := owner[e.f]; !taken || try(o, seen) {
					owner[e.f] = i
					return true
				}
			}
			return false
		}
		for _, i := range jobs {
			try(i, map[int]bool{})
		}
		return owner
	}

	// Every job can reach every freighter, so the best size is the smaller
	// count. Binary search the sorted distances for the smallest limit that
	// reaches it.
	want := min(len(jobs), len(fs))
	slices.Sort(dists)
	lo, hi := 0, len(dists)-1
	for lo < hi {
		mid := (lo + hi) / 2
		if len(match(dists[mid])) >= want {
			hi = mid
		} else {
			lo = mid + 1
		}
	}

	out := map[int]dispatch{}
	for f, i := range match(dists[lo]) {
		out[i] = dispatch{Freighter: fs[f], Dist: fs[f].Pos.Distance(starPos[as[i].Star])}
	}
	return out
}
