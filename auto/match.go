package auto

import "math"

// MatchMinMax pairs agents with tasks, one task per agent and one agent per
// task, making as many pairs as the smaller side has. It minimises the
// longest distance in any pair, then the total. It returns, for each agent,
// the index of its task, or -1 if it gets none. The search is exhaustive
// (with pruning), so keep the numbers small.
func MatchMinMax(nAgents, nTasks int, dist func(agent, task int) float64) []int {
	const eps = 1e-9
	want := min(nAgents, nTasks)
	best := make([]int, nAgents)
	for a := range best {
		best[a] = -1
	}
	bestMax, bestSum := math.Inf(1), math.Inf(1)
	cur := make([]int, nAgents)
	used := make([]bool, nTasks)
	var rec func(a, pairs int, curMax, sum float64)
	rec = func(a, pairs int, curMax, sum float64) {
		if curMax > bestMax+eps || (curMax > bestMax-eps && sum >= bestSum-eps) {
			return
		}
		if pairs == want {
			for i := a; i < nAgents; i++ {
				cur[i] = -1
			}
			copy(best, cur)
			bestMax, bestSum = curMax, sum
			return
		}
		if a == nAgents {
			return
		}
		for t := range nTasks {
			if used[t] {
				continue
			}
			used[t], cur[a] = true, t
			d := dist(a, t)
			rec(a+1, pairs+1, max(curMax, d), sum+d)
			used[t] = false
		}
		// This agent can sit out if the rest can still make enough pairs.
		if pairs+nAgents-a-1 >= want {
			cur[a] = -1
			rec(a+1, pairs, curMax, sum)
		}
	}
	rec(0, 0, 0, 0)
	return best
}
