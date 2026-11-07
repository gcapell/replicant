package auto

import (
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

type pt struct {
	Name string
	Pos  model.Position
}

func ptPos(p pt) model.Position { return p.Pos }

func p(name string, x, y float64) pt { return pt{name, model.Position{X: x, Y: y}} }

func ptNames(ps []pt) []string {
	var n []string
	for _, x := range ps {
		n = append(n, x.Name)
	}
	return n
}

func TestSeedsMaxMin(t *testing.T) {
	// A, B and C are far apart; A2 is next to A. Max-sum might pick A and A2
	// with the far one; max-min must not.
	pts := []pt{p("A", 0, 0), p("A2", 1, 0), p("B", 100, 0), p("C", 50, 80)}
	var n []string
	for _, i := range Seeds(pts, 3, ptPos) {
		n = append(n, pts[i].Name)
	}
	if !slices.Contains(n, "B") || !slices.Contains(n, "C") || (slices.Contains(n, "A") == slices.Contains(n, "A2")) {
		t.Errorf("seeds = %v, want B, C and one of A/A2", n)
	}
	if ff := farthestFirst(pts, 3, ptPos); len(ff) != 3 {
		t.Errorf("farthestFirst = %v", ff)
	}
	if s := Seeds(pts, 0, ptPos); s != nil {
		t.Errorf("k=0: %v", s)
	}
}

func TestGroupNearest(t *testing.T) {
	pts := []pt{p("L", 0, 0), p("R", 100, 0), p("L1", 40, 0), p("R1", 60, 0), p("R2", 99, 0)}
	g := GroupNearest(pts, []int{0, 1}, ptPos)
	if a, b := ptNames(g[0]), ptNames(g[1]); !slices.Equal(a, []string{"L", "L1"}) || !slices.Equal(b, []string{"R", "R1", "R2"}) {
		t.Errorf("groups = %v %v", a, b)
	}
}

func TestAssign(t *testing.T) {
	groups := [][]pt{{p("L", 0, 0)}, {p("R", 100, 0)}}
	agents := []pt{p("near-R", 90, 0), p("far", 500, 0), p("near-L", 10, 0)}
	if got := Assign(agents, ptPos, groups, ptPos); !slices.Equal(got, []int{2, 0}) {
		t.Errorf("assign = %v, want [2 0]", got)
	}
}

func TestOrderStartsNearest(t *testing.T) {
	g := []pt{p("C", 30, 0), p("A", 10, 0), p("B", 20, 0)}
	if got := ptNames(Order(model.Position{}, g, ptPos)); !slices.Equal(got, []string{"A", "B", "C"}) {
		t.Errorf("order = %v", got)
	}
}
