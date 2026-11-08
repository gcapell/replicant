package main

import (
	"slices"
	"testing"
)

func codes(n int) []string {
	var out []string
	for i := range n {
		out = append(out, string(rune('a'+i)))
	}
	return out
}

func TestNewTeam(t *testing.T) {
	v := vessel{Code: "V", Pos: xy(0, 0), Relays: []string{"s1"}}
	tm := newTeam(v, fetch{Loc: "B-BELT-1", Pos: xy(5, 0), Dist: 5, Relays: relays("f1", "f2")})
	if tm.Start != xy(5, 0) || !slices.Equal(tm.Relays, []string{"s1", "f1", "f2"}) {
		t.Errorf("team = %+v", tm)
	}
	if tm := newTeam(v, fetch{}); tm.Start != xy(0, 0) {
		t.Errorf("no-fetch team starts at %v", tm.Start)
	}
}

func TestDivideTwoClusters(t *testing.T) {
	teams := []team{
		newTeam(vessel{Code: "L", Pos: xy(-10, 0), Relays: codes(5)}, fetch{}),
		newTeam(vessel{Code: "R", Pos: xy(10, 0), Relays: codes(5)}, fetch{}),
	}
	targets := []star{at("L1", -9, 0), at("R1", 9, 0), at("L2", -8, 1), at("R2", 8, 1), at("L3", -7, 0)}
	if left := divide(teams, targets); len(left) != 0 {
		t.Fatalf("left over: %v", names(left))
	}
	l, r := names(teams[0].Route), names(teams[1].Route)
	slices.Sort(l)
	slices.Sort(r)
	if !slices.Equal(l, []string{"L1", "L2", "L3"}) || !slices.Equal(r, []string{"R1", "R2"}) {
		t.Errorf("L = %v, R = %v", l, r)
	}
}

func TestDivideRespectsCapacity(t *testing.T) {
	teams := []team{
		newTeam(vessel{Code: "SMALL", Pos: xy(0, 0), Relays: codes(1)}, fetch{}),
		newTeam(vessel{Code: "BIG", Pos: xy(50, 0), Relays: codes(5)}, fetch{}),
	}
	targets := []star{at("A", 1, 0), at("B", 2, 0), at("C", 3, 0), at("D", 49, 0)}
	if left := divide(teams, targets); len(left) != 0 {
		t.Fatalf("left over: %v", names(left))
	}
	if len(teams[0].Route) != 1 || len(teams[1].Route) != 3 {
		t.Errorf("SMALL = %v, BIG = %v", names(teams[0].Route), names(teams[1].Route))
	}
}

func TestDivideCountsFetchInCost(t *testing.T) {
	// Both start next to the stars, but FETCHER travelled 100 to get there,
	// so NEAR should take the rest.
	teams := []team{
		newTeam(vessel{Code: "NEAR", Pos: xy(0, 0), Relays: codes(5)}, fetch{}),
		newTeam(vessel{Code: "FETCHER", Pos: xy(100, 0)}, fetch{Loc: "B-BELT-1", Pos: xy(20, 0), Dist: 100, Relays: relays("x", "y", "z")}),
	}
	targets := []star{at("A", 0, 1), at("B", 20, 1), at("C", 10, 0), at("D", 11, 0)}
	divide(teams, targets)
	if got := names(teams[1].Route); len(got) != 1 {
		t.Errorf("FETCHER route = %v, want just its seed", got)
	}
}

func TestDivideMoreTeamsThanStars(t *testing.T) {
	teams := []team{
		newTeam(vessel{Code: "A", Pos: xy(0, 0), Relays: codes(2)}, fetch{}),
		newTeam(vessel{Code: "B", Pos: xy(10, 0), Relays: codes(2)}, fetch{}),
		newTeam(vessel{Code: "EMPTY", Pos: xy(1, 0)}, fetch{}),
	}
	divide(teams, []star{at("S", 9, 0)})
	if len(teams[0].Route) != 0 || len(teams[1].Route) != 1 || len(teams[2].Route) != 0 {
		t.Errorf("routes = %v %v %v", names(teams[0].Route), names(teams[1].Route), names(teams[2].Route))
	}
}
