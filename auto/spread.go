package auto

import (
	"math"
	"slices"

	"github.com/gcapell/replicant/model"
)

// Splitting work among several devices: pick spread-out seeds, group the
// rest around them, give each group a device, and order each group into a
// route. pos gives each item's position.

// maxCombos is the most seed sets Seeds tries exhaustively before falling
// back to picking seeds greedily.
const maxCombos = 200_000

// Seeds picks k items (by index) that are as spread out as possible: it
// maximises the smallest distance between any two of them, breaking ties by
// the total distance between them. With too many combinations to try, it
// picks greedily (farthest first).
func Seeds[T any](items []T, k int, pos func(T) model.Position) []int {
	if k <= 0 || len(items) == 0 {
		return nil
	}
	if k == 1 {
		return []int{0}
	}
	if combos(len(items), k) > maxCombos {
		return farthestFirst(items, k, pos)
	}
	var best []int
	bestMin, bestSum := -1.0, -1.0
	cur := make([]int, 0, k)
	var rec func(from int)
	rec = func(from int) {
		if len(cur) == k {
			mn, sum := spread(items, cur, pos)
			if mn > bestMin+1e-9 || (mn > bestMin-1e-9 && sum > bestSum+1e-9) {
				best, bestMin, bestSum = slices.Clone(cur), mn, sum
			}
			return
		}
		for i := from; i <= len(items)-(k-len(cur)); i++ {
			cur = append(cur, i)
			rec(i + 1)
			cur = cur[:len(cur)-1]
		}
	}
	rec(0)
	return best
}

// spread returns the smallest and the total pairwise distance between the
// chosen items.
func spread[T any](items []T, chosen []int, pos func(T) model.Position) (mn, sum float64) {
	mn = math.Inf(1)
	for i, a := range chosen {
		for _, b := range chosen[i+1:] {
			d := pos(items[a]).Distance(pos(items[b]))
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

// farthestFirst starts from the two items farthest apart, then repeatedly
// adds the item farthest from those already chosen.
func farthestFirst[T any](items []T, k int, pos func(T) model.Position) []int {
	a, b, far := 0, 0, -1.0
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			if d := pos(items[i]).Distance(pos(items[j])); d > far {
				a, b, far = i, j, d
			}
		}
	}
	chosen := []int{a, b}
	for len(chosen) < k {
		next, nextD := -1, -1.0
		for i, it := range items {
			if slices.Contains(chosen, i) {
				continue
			}
			d := math.Inf(1)
			for _, c := range chosen {
				d = min(d, pos(it).Distance(pos(items[c])))
			}
			if d > nextD {
				next, nextD = i, d
			}
		}
		chosen = append(chosen, next)
	}
	return chosen
}

// GroupNearest puts each seed in its own group, then every other item in the
// group of its nearest seed.
func GroupNearest[T any](items []T, seeds []int, pos func(T) model.Position) [][]T {
	groups := make([][]T, len(seeds))
	for g, i := range seeds {
		groups[g] = []T{items[i]}
	}
	for i, it := range items {
		if slices.Contains(seeds, i) {
			continue
		}
		best := 0
		for g, s := range seeds {
			if pos(it).Distance(pos(items[s])) < pos(it).Distance(pos(items[seeds[best]])) {
				best = g
			}
		}
		groups[best] = append(groups[best], it)
	}
	return groups
}

// DistToSet is the distance from p to the nearest item in set.
func DistToSet[T any](p model.Position, set []T, pos func(T) model.Position) float64 {
	d := math.Inf(1)
	for _, s := range set {
		d = min(d, p.Distance(pos(s)))
	}
	return d
}

// Assign gives each group a different agent, minimising the total distance
// from each agent to the nearest item in its group (exhaustively, so keep
// the numbers small). It returns, for each group, the index of its agent.
// There must be at least as many agents as groups.
func Assign[A, T any](agents []A, apos func(A) model.Position, groups [][]T, pos func(T) model.Position) []int {
	var best []int
	bestCost := math.Inf(1)
	cur := make([]int, 0, len(groups))
	used := make([]bool, len(agents))
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
		for a := range agents {
			if used[a] {
				continue
			}
			used[a] = true
			cur = append(cur, a)
			rec(cost + DistToSet(apos(agents[a]), groups[g], pos))
			cur = cur[:len(cur)-1]
			used[a] = false
		}
	}
	rec(0)
	return best
}

// Order visits the group's nearest item first, then the rest by a short
// path from there.
func Order[T any](start model.Position, group []T, pos func(T) model.Position) []T {
	if len(group) == 0 {
		return nil
	}
	first := 0
	for i, it := range group {
		if start.Distance(pos(it)) < start.Distance(pos(group[first])) {
			first = i
		}
	}
	rest := slices.Delete(slices.Clone(group), first, first+1)
	return append([]T{group[first]}, ShortRoute(pos(group[first]), rest, pos)...)
}
