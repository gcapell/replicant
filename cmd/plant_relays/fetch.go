package main

import (
	"math"
	"slices"

	"github.com/gcapell/replicant/model"
)

// Phase 1: decide which vessels fetch ftl_relays, and from which belt.

// vessel is an idle heaven_vessel carrying a replicant.
type vessel struct {
	Code, Replicant, Loc string
	Pos                  model.Position // its star's
	Relays               []string       // ftl_relays already stowed
	Free                 int            // free stow slots
}

// relay is an ftl_relay waiting at a belt.
type relay struct{ Code, Owner string }

// belt is a -BELT- location with ftl_relays waiting there.
type belt struct {
	Loc    string
	Pos    model.Position // its star's
	Relays []relay
}

// fetch is one vessel's trip to collect relays. Loc is "" for no trip.
type fetch struct {
	Loc    string
	Pos    model.Position
	Dist   float64 // from the vessel
	Relays []relay // what it picks up
}

// fetchOptions is how many of its nearest belts each vessel considers.
const fetchOptions = 4

// maxFetchPlans bounds the exhaustive search in planFetch.
const maxFetchPlans = 1_000_000

// planFetch decides, for each vessel, whether to fetch relays and from which
// belt, so that the vessels end up holding at least need relays between
// them. A vessel that goes to a belt takes all the relays there that it has
// room for (vessels sharing a belt take them in order). In order of
// importance, the plan:
//
//  1. falls as little short of need as possible;
//  2. leaves as few vessels as possible with no relays (up to need vessels);
//  3. keeps the longest trip to a belt short;
//  4. keeps the total distance to belts short.
//
// Each vessel considers only no trip or its nearest few belts, and the
// combinations are tried exhaustively.
func planFetch(vs []vessel, belts []belt, need int) []fetch {
	k := fetchOptions
	for k > 1 && math.Pow(float64(k+1), float64(len(vs))) > maxFetchPlans {
		k--
	}
	opts := make([][]int, len(vs))
	for i, v := range vs {
		opts[i] = []int{-1}
		if v.Free <= 0 {
			continue
		}
		var near []int
		for b := range belts {
			if len(belts[b].Relays) > 0 {
				near = append(near, b)
			}
		}
		slices.SortStableFunc(near, func(a, b int) int {
			return cmpFloat(v.Pos.Distance(belts[a].Pos), v.Pos.Distance(belts[b].Pos))
		})
		opts[i] = append(opts[i], near[:min(k, len(near))]...)
	}

	cur := make([]int, len(vs))
	var best []fetch
	var bestScore fetchScore
	var rec func(i int)
	rec = func(i int) {
		if i == len(vs) {
			fs := fetches(vs, belts, cur)
			if s := score(vs, fs, need); best == nil || s.less(bestScore) {
				best, bestScore = fs, s
			}
			return
		}
		for _, b := range opts[i] {
			cur[i] = b
			rec(i + 1)
		}
	}
	rec(0)
	return best
}

// fetches works out what each vessel picks up if vessel i goes to belt
// choice[i] (-1 for none). A vessel that would pick up nothing stays put.
func fetches(vs []vessel, belts []belt, choice []int) []fetch {
	taken := make([]int, len(belts))
	fs := make([]fetch, len(vs))
	for i, b := range choice {
		if b < 0 {
			continue
		}
		n := min(vs[i].Free, len(belts[b].Relays)-taken[b])
		if n <= 0 {
			continue
		}
		fs[i] = fetch{
			Loc:    belts[b].Loc,
			Pos:    belts[b].Pos,
			Dist:   vs[i].Pos.Distance(belts[b].Pos),
			Relays: belts[b].Relays[taken[b] : taken[b]+n],
		}
		taken[b] += n
	}
	return fs
}

type fetchScore struct {
	short, empty int
	longest, sum float64
}

func score(vs []vessel, fs []fetch, need int) fetchScore {
	var s fetchScore
	total, holding := 0, 0
	for i, f := range fs {
		n := len(vs[i].Relays) + len(f.Relays)
		total += n
		if n > 0 {
			holding++
		}
		s.longest = max(s.longest, f.Dist)
		s.sum += f.Dist
	}
	s.short = max(0, need-total)
	s.empty = max(0, min(len(vs), need)-holding)
	return s
}

func (a fetchScore) less(b fetchScore) bool {
	const eps = 1e-9
	switch {
	case a.short != b.short:
		return a.short < b.short
	case a.empty != b.empty:
		return a.empty < b.empty
	case math.Abs(a.longest-b.longest) > eps:
		return a.longest < b.longest
	default:
		return a.sum < b.sum-eps
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
