package auto

import (
	"math"
	"slices"

	"github.com/gcapell/replicant/model"
)

// RelayReach is how close (in light years) a star must be to a relay star
// to join the relay mesh: ftl_relay's relay_range.
const RelayReach = 7.5

// Connectable returns the candidates that can join the mesh: those within
// RelayReach (strictly) of a mesh star, or of a candidate that can. They come
// in the order the mesh would grow, nearest to it first, so any prefix of the
// result is itself connected to the mesh.
func Connectable[T any](mesh, candidates []T, pos func(T) model.Position) []T {
	covered := slices.Clone(mesh)
	left := slices.Clone(candidates)
	var out []T
	for {
		best, bestD := -1, math.Inf(1)
		for i, c := range left {
			if d := DistToSet(pos(c), covered, pos); d < RelayReach && d < bestD {
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
