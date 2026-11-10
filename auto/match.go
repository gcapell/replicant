package auto

import "math"

// MatchMinMax pairs agents with tasks, one task per agent and one agent per
// task. A pair whose distance is +Inf is impossible. It makes as many pairs
// as it can, then minimises the longest distance in any pair, then the
// total. It returns, for each agent, the index of its task, or -1 if it gets
// none. The search is exhaustive (with pruning), so keep the numbers small.
func MatchMinMax(nAgents, nTasks int, dist func(agent, task int) float64) []int {
	const eps = 1e-9
	best := make([]int, nAgents)
	for a := range best {
		best[a] = -1
	}
	bestPairs, bestMax, bestSum := 0, math.Inf(1), math.Inf(1)
	// worse reports whether (max, sum) is no better than the best so far.
	worse := func(curMax, sum float64) bool {
		return curMax > bestMax+eps || (curMax > bestMax-eps && sum >= bestSum-eps)
	}
	cur := make([]int, nAgents)
	used := make([]bool, nTasks)
	var rec func(a, pairs int, curMax, sum float64)
	rec = func(a, pairs int, curMax, sum float64) {
		// At most this many pairs are still possible.
		if bound := pairs + min(nAgents-a, nTasks-pairs); bound < bestPairs || (bound == bestPairs && worse(curMax, sum)) {
			return
		}
		if a == nAgents {
			// The bound check above means this is an improvement.
			copy(best, cur)
			bestPairs, bestMax, bestSum = pairs, curMax, sum
			return
		}
		for t := range nTasks {
			if used[t] {
				continue
			}
			d := dist(a, t)
			if math.IsInf(d, 1) {
				continue
			}
			used[t], cur[a] = true, t
			rec(a+1, pairs+1, max(curMax, d), sum+d)
			used[t] = false
		}
		cur[a] = -1
		rec(a+1, pairs, curMax, sum)
	}
	rec(0, 0, 0, 0)
	return best
}
