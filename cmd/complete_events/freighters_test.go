package main

import (
	"testing"

	"github.com/gcapell/replicant/model"
)

func job(star string) assignment { return assignment{Star: star} }

func fr(code string, x float64) freighter { return freighter{Code: code, Pos: pos(x)} }

func codes(as []assignment, ds map[int]dispatch) map[string]string {
	m := map[string]string{}
	for i, d := range ds {
		m[as[i].Star] = d.Freighter.Code
	}
	return m
}

func TestAssignMinimisesMaxDistance(t *testing.T) {
	// Stars at 0 and 10. With freighters at 6 and 20, A-F6, B-F20 (max 10)
	// beats A-F20, B-F6 (max 20).
	starPos := map[string]model.Position{"A": pos(0), "B": pos(10)}
	as := []assignment{job("A"), job("B")}
	got := codes(as, assignFreighters(as, []freighter{fr("F20", 20), fr("F6", 6)}, starPos))
	if got["A"] != "F6" || got["B"] != "F20" {
		t.Errorf("got %v", got)
	}

	// Nearest-first for A would grab F1, forcing B onto F-9 (19 away).
	// Bottleneck: A-F-9 (9), B-F1 (9).
	got = codes(as, assignFreighters(as, []freighter{fr("F1", 1), fr("F-9", -9)}, starPos))
	if got["A"] != "F-9" || got["B"] != "F1" {
		t.Errorf("got %v", got)
	}
}

func TestAssignFewerFreighters(t *testing.T) {
	// One freighter, two jobs: it goes to the nearer one.
	starPos := map[string]model.Position{"A": pos(0), "B": pos(10)}
	as := []assignment{job("A"), job("B"), {}} // the last has no source
	ds := assignFreighters(as, []freighter{fr("F", 8)}, starPos)
	if len(ds) != 1 || ds[1].Freighter.Code != "F" || ds[1].Dist != 2 {
		t.Errorf("got %+v", ds)
	}
}

func TestAssignMoreFreighters(t *testing.T) {
	starPos := map[string]model.Position{"A": pos(0)}
	as := []assignment{job("A")}
	ds := assignFreighters(as, []freighter{fr("F5", 5), fr("F1", 1), fr("F3", 3)}, starPos)
	if len(ds) != 1 || ds[0].Freighter.Code != "F1" {
		t.Errorf("got %+v", ds)
	}
}

func TestAssignNothing(t *testing.T) {
	if ds := assignFreighters([]assignment{{}}, []freighter{fr("F", 0)}, nil); len(ds) != 0 {
		t.Errorf("got %+v", ds)
	}
	if ds := assignFreighters([]assignment{job("A")}, nil, nil); len(ds) != 0 {
		t.Errorf("got %+v", ds)
	}
}
