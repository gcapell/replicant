package auto

import (
	"math"
	"slices"
	"testing"
)

func TestMatchMinMax(t *testing.T) {
	line := func(agents, tasks []float64) func(a, t int) float64 {
		return func(a, t int) float64 { return math.Abs(agents[a] - tasks[t]) }
	}
	for _, tc := range []struct {
		name          string
		agents, tasks []float64
		want          []int
	}{
		{"none", nil, nil, []int{}},
		{"no tasks", []float64{0, 1}, nil, []int{-1, -1}},
		{"no agents", nil, []float64{0}, []int{}},
		{"nearest", []float64{0, 10}, []float64{9, 1}, []int{1, 0}},
		{"more tasks", []float64{5}, []float64{0, 4, 9}, []int{1}},
		{"more agents", []float64{0, 5, 9}, []float64{4}, []int{-1, 0, -1}},
		{"more agents, two tasks", []float64{0, 5, 9}, []float64{1, 10}, []int{0, -1, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchMinMax(len(tc.agents), len(tc.tasks), line(tc.agents, tc.tasks))
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	// Pairing 0-0 and 1-1 has the smaller total (1+9 = 10 against 6+5 = 11),
	// but the longer longest trip (9 against 6).
	m := [][]float64{{1, 6}, {5, 9}}
	got := MatchMinMax(2, 2, func(a, t int) float64 { return m[a][t] })
	if want := []int{1, 0}; !slices.Equal(got, want) {
		t.Errorf("min max over min total: got %v, want %v", got, want)
	}

	// Both ways have a longest trip of 10; the second has the smaller total.
	m = [][]float64{{10, 10}, {1, 10}}
	got = MatchMinMax(2, 2, func(a, t int) float64 { return m[a][t] })
	if want := []int{1, 0}; !slices.Equal(got, want) {
		t.Errorf("total breaks ties: got %v, want %v", got, want)
	}

	// Impossible pairs (+Inf). Pairing agent 0 with task 0 (distance 1)
	// leaves agent 1 nothing it can do; more pairs beats a shorter trip.
	inf := math.Inf(1)
	m = [][]float64{{1, 5}, {2, inf}}
	got = MatchMinMax(2, 2, func(a, t int) float64 { return m[a][t] })
	if want := []int{1, 0}; !slices.Equal(got, want) {
		t.Errorf("most pairs first: got %v, want %v", got, want)
	}
	// A task nobody can do stays unmatched.
	m = [][]float64{{inf, 3}, {inf, 1}}
	got = MatchMinMax(2, 2, func(a, t int) float64 { return m[a][t] })
	if want := []int{-1, 1}; !slices.Equal(got, want) {
		t.Errorf("impossible task: got %v, want %v", got, want)
	}
	m = [][]float64{{inf}}
	if got = MatchMinMax(1, 1, func(a, t int) float64 { return m[a][t] }); !slices.Equal(got, []int{-1}) {
		t.Errorf("nothing possible: got %v", got)
	}
}
