package main

import (
	"testing"
)

func relays(codes ...string) []relay {
	var rs []relay
	for _, c := range codes {
		rs = append(rs, relay{Code: c})
	}
	return rs
}

func fetchLocs(fs []fetch) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Loc)
	}
	return out
}

func TestPlanFetchNoneNeeded(t *testing.T) {
	vs := []vessel{{Code: "V", Relays: []string{"a", "b"}, Free: 5}}
	belts := []belt{{Loc: "B-BELT-1", Pos: xy(1, 0), Relays: relays("x")}}
	if fs := planFetch(vs, belts, 2); fs[0].Loc != "" {
		t.Errorf("fetched %+v, want nothing", fs)
	}
}

func TestPlanFetchTakesAllItCanFit(t *testing.T) {
	vs := []vessel{{Code: "V", Relays: []string{"a"}, Free: 3}}
	belts := []belt{{Loc: "B-BELT-1", Pos: xy(1, 0), Relays: relays("x", "y", "z", "w")}}
	fs := planFetch(vs, belts, 2)
	if fs[0].Loc != "B-BELT-1" || len(fs[0].Relays) != 3 {
		t.Errorf("fetch = %+v, want 3 from B-BELT-1", fs[0])
	}
}

func TestPlanFetchMinimisesLongestTrip(t *testing.T) {
	// Need 4 more. Sending both vessels to their own nearby belts (longest 2)
	// beats sending one far to the big belt (10).
	vs := []vessel{
		{Code: "V1", Pos: xy(0, 0), Free: 5},
		{Code: "V2", Pos: xy(20, 0), Free: 5},
	}
	belts := []belt{
		{Loc: "A-BELT-1", Pos: xy(1, 0), Relays: relays("a1", "a2")},
		{Loc: "BIG-BELT-1", Pos: xy(10, 0), Relays: relays("b1", "b2", "b3", "b4")},
		{Loc: "C-BELT-1", Pos: xy(22, 0), Relays: relays("c1", "c2")},
	}
	fs := planFetch(vs, belts, 4)
	if got := fetchLocs(fs); got[0] != "A-BELT-1" || got[1] != "C-BELT-1" {
		t.Errorf("fetches = %v", got)
	}
}

func TestPlanFetchEveryVesselGetsRelays(t *testing.T) {
	// V1 already holds enough, but V2 has none, so it fetches even though
	// the belt is far.
	vs := []vessel{
		{Code: "V1", Pos: xy(0, 0), Relays: []string{"r1", "r2", "r3"}, Free: 5},
		{Code: "V2", Pos: xy(0, 0), Free: 5},
	}
	belts := []belt{{Loc: "FAR-BELT-1", Pos: xy(100, 0), Relays: relays("f1")}}
	fs := planFetch(vs, belts, 3)
	if got := fetchLocs(fs); got[0] != "" || got[1] != "FAR-BELT-1" {
		t.Errorf("fetches = %v", got)
	}
}

func TestPlanFetchSharedBelt(t *testing.T) {
	// Both vessels must use the one belt, and they split its relays.
	vs := []vessel{{Code: "V1", Free: 2}, {Code: "V2", Free: 5}}
	belts := []belt{{Loc: "B-BELT-1", Pos: xy(1, 0), Relays: relays("a", "b", "c", "d")}}
	fs := planFetch(vs, belts, 4)
	if len(fs[0].Relays)+len(fs[1].Relays) != 4 || len(fs[0].Relays) == 0 || len(fs[1].Relays) == 0 {
		t.Errorf("fetches = %+v", fs)
	}
	seen := map[string]bool{}
	for _, f := range fs {
		for _, r := range f.Relays {
			if seen[r.Code] {
				t.Errorf("%s fetched twice", r.Code)
			}
			seen[r.Code] = true
		}
	}
}

func TestPlanFetchShortfall(t *testing.T) {
	// Not enough anywhere: take everything.
	vs := []vessel{{Code: "V", Free: 10}}
	belts := []belt{
		{Loc: "A-BELT-1", Pos: xy(1, 0), Relays: relays("a")},
		{Loc: "B-BELT-1", Pos: xy(2, 0), Relays: relays("b", "c")},
	}
	fs := planFetch(vs, belts, 10)
	if fs[0].Loc != "B-BELT-1" {
		t.Errorf("fetch = %+v, want the bigger belt", fs[0])
	}
}

func TestPlanFetchFullHold(t *testing.T) {
	vs := []vessel{{Code: "V", Free: 0}}
	belts := []belt{{Loc: "A-BELT-1", Relays: relays("a")}}
	if fs := planFetch(vs, belts, 5); fs[0].Loc != "" {
		t.Errorf("fetch = %+v, want none", fs[0])
	}
}
