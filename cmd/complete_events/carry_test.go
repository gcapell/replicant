package main

import (
	"slices"
	"testing"
)

func TestVisits(t *testing.T) {
	spares := []spare{{Code: "B1"}, {Code: "B2"}, {Code: "G"}}
	r := []stop{
		{At: at("S-BELT-1", 0), Spare: 0, Pickup: true},
		{At: at("S-BELT-1", 0), Spare: 1, Pickup: true},
		{At: at("E-1", 5), Spare: 0},
		{At: at("E-1", 5), Spare: 2, Pickup: true},
		{At: at("F-2", 9), Spare: 1},
		{At: at("F-2", 9), Spare: 2},
	}
	got := visits(r, spares)
	want := []visit{
		{Loc: "S-BELT-1", Attach: []string{"B1", "B2"}},
		{Loc: "E-1", Detach: []string{"B1"}, Attach: []string{"G"}},
		{Loc: "F-2", Detach: []string{"B2", "G"}},
	}
	if !slices.EqualFunc(got, want, func(a, b visit) bool {
		return a.Loc == b.Loc && slices.Equal(a.Attach, b.Attach) && slices.Equal(a.Detach, b.Detach)
	}) {
		t.Errorf("visits = %+v", got)
	}
}
