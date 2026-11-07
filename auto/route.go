package auto

import (
	"slices"

	"github.com/gcapell/replicant/model"
)

// ShortRoute orders points into a short open path from start (visiting each
// once, not returning). It takes the nearest unvisited point each time, then
// improves the order with 2-opt moves. pos gives each point's position.
func ShortRoute[T any](start model.Position, points []T, pos func(T) model.Position) []T {
	left := slices.Clone(points)
	route := make([]T, 0, len(points))
	at := start
	for len(left) > 0 {
		best := 0
		for i := range left {
			if at.Distance(pos(left[i])) < at.Distance(pos(left[best])) {
				best = i
			}
		}
		route = append(route, left[best])
		at = pos(left[best])
		left = slices.Delete(left, best, best+1)
	}
	return twoOpt(start, route, pos)
}

// RouteLength is the distance from start through each point in turn.
func RouteLength[T any](start model.Position, route []T, pos func(T) model.Position) float64 {
	total, at := 0.0, start
	for _, p := range route {
		total += at.Distance(pos(p))
		at = pos(p)
	}
	return total
}

func twoOpt[T any](start model.Position, route []T, pos func(T) model.Position) []T {
	const eps = 1e-9
	best := RouteLength(start, route, pos)
	for improved := true; improved; {
		improved = false
		for i := range len(route) - 1 {
			for j := i + 1; j < len(route); j++ {
				slices.Reverse(route[i : j+1])
				if l := RouteLength(start, route, pos); l < best-eps {
					best, improved = l, true
				} else {
					slices.Reverse(route[i : j+1])
				}
			}
		}
	}
	return route
}
