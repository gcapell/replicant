package main

import (
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

const (
	C  = model.Carbon
	Si = model.Silicates
)

func pos(x float64) model.Position { return model.Position{X: x} }

func TestToNeedUsesChosenOption(t *testing.T) {
	ev := &model.LocationEvent{Progress: model.EventProgress{Options: []model.EventOption{
		{
			Resources: []model.RequiredResource{
				{ResourceType: C, Current: 20, Required: 50},
				{ResourceType: Si, Current: 100, Required: 100},
			},
			Devices: []model.RequiredDevice{{DeviceType: "defence_grid", Required: 1}},
		},
		{Resources: []model.RequiredResource{{ResourceType: model.Rares, Required: 5}}},
	}}}
	n := toNeed("PARBEA-5", ev, 0)
	if n.Star != "PARBEA" || len(n.Want) != 1 || n.Want[C] != 30 {
		t.Errorf("need = %+v", n)
	}
	if len(n.Devices) != 1 || n.Devices[0].DeviceType != "defence_grid" {
		t.Errorf("devices = %+v", n.Devices)
	}
	if n := toNeed("PARBEA-5", ev, 1); n.Option != 1 || n.Want[model.Rares] != 5 || len(n.Devices) != 0 {
		t.Errorf("option 1: %+v", n)
	}
	if n := toNeed("X-1", &model.LocationEvent{}, 0); len(n.Want) != 0 || n.Met {
		t.Errorf("no options: %+v", n)
	}
}

func TestIsSource(t *testing.T) {
	for loc, want := range map[string]bool{
		"A-3": true, "A-3-1": true, "A-BELT-1": true,
		"A-3-L4": false, "A-OBJ-77": false, "A-BELT-1-X": false, "A-OORT": false, "A": false,
	} {
		if got := isSource(loc); got != want {
			t.Errorf("isSource(%q) = %v", loc, got)
		}
	}
}

func TestPlanNearestCoveringStar(t *testing.T) {
	// NEAR has carbon but not enough silicates; FAR has both, split over
	// two locations.
	stock := map[string]amounts{
		"NEAR-1":     {C: 100, Si: 10},
		"FAR-2":      {C: 100},
		"FAR-BELT-1": {C: 10, Si: 200},
	}
	starPos := map[string]model.Position{"NEAR": pos(1), "FAR": pos(10)}
	needs := []need{{Loc: "EV-5", Pos: pos(0), Want: amounts{C: 50, Si: 100}}}

	got := plan(needs, stock, starPos)
	if len(got) != 1 || got[0].Star != "FAR" || got[0].Dist != 10 {
		t.Fatalf("plan = %+v", got)
	}
	want := []pickup{{"FAR-2", C, 50}, {"FAR-BELT-1", Si, 100}}
	if !slices.Equal(got[0].Collect, want) {
		t.Errorf("collect = %+v, want %+v", got[0].Collect, want)
	}
	if stock["FAR-2"][C] != 50 || stock["FAR-BELT-1"][Si] != 100 {
		t.Errorf("stock not consumed: %+v", stock)
	}
}

func TestPlanSplitsAcrossLocations(t *testing.T) {
	stock := map[string]amounts{"S-1": {C: 30}, "S-2": {C: 40}}
	starPos := map[string]model.Position{"S": pos(0)}
	got := plan([]need{{Loc: "S-3", Pos: pos(0), Want: amounts{C: 60}}}, stock, starPos)
	want := []pickup{{"S-2", C, 40}, {"S-1", C, 20}}
	if len(got) != 1 || !slices.Equal(got[0].Collect, want) {
		t.Errorf("plan = %+v", got)
	}
}

func TestPlanNoDoubleCounting(t *testing.T) {
	// One stockpile of 100 carbon, two events wanting 80 each. The event
	// nearer the stock gets it; the other has no source.
	stock := map[string]amounts{"S-1": {C: 100}}
	starPos := map[string]model.Position{"S": pos(0)}
	needs := []need{
		{Loc: "A-1", Pos: pos(5), Want: amounts{C: 80}},
		{Loc: "B-1", Pos: pos(2), Want: amounts{C: 80}},
	}
	got := plan(needs, stock, starPos)
	if len(got) != 2 || got[0].Need.Loc != "B-1" || got[0].Star != "S" {
		t.Fatalf("plan = %+v", got)
	}
	if got[1].Need.Loc != "A-1" || got[1].Star != "" || got[1].Nearest[C] != "" {
		t.Errorf("second = %+v", got[1])
	}
}

func TestPlanNoSingleSourceNearestPerResource(t *testing.T) {
	stock := map[string]amounts{"CS-1": {C: 100}, "SS-1": {Si: 100}}
	starPos := map[string]model.Position{"CS": pos(1), "SS": pos(2)}
	got := plan([]need{{Loc: "E-1", Pos: pos(0), Want: amounts{C: 10, Si: 10}}}, stock, starPos)
	if len(got) != 1 || got[0].Star != "" || got[0].Nearest[C] != "CS" || got[0].Nearest[Si] != "SS" {
		t.Errorf("plan = %+v", got)
	}
}

func TestPlanSkipsMetAndEmpty(t *testing.T) {
	needs := []need{
		{Loc: "A-1", Met: true},
		{Loc: "B-1", Want: amounts{}},
	}
	if got := plan(needs, map[string]amounts{}, nil); len(got) != 0 {
		t.Errorf("plan = %+v", got)
	}
}

func TestDescribe(t *testing.T) {
	as := []assignment{{
		Need:    need{Loc: "PARBEA-5", Want: amounts{C: 50, Si: 100}, Beacon: true, Relay: true},
		Star:    "OTHYONAL",
		Dist:    4.25,
		Collect: []pickup{{"OTHYONAL-3", C, 50}, {"OTHYONAL-3", Si, 60}, {"OTHYONAL-BELT-1", Si, 40}},
	}}
	as = append(as, assignment{Need: need{Loc: "Q-1", Want: amounts{C: 1}, Beacon: true, Relay: true}, Star: "S", Collect: []pickup{{"S-1", C, 1}}})
	ds := map[int]dispatch{0: {Freighter: freighter{Code: "F1"}, Dist: 4.5}}
	needs := []need{{Loc: "Z-1", Met: true, Relay: true}}
	got := describe(as, ds, needs)
	want := []string{
		"PARBEA-5 requires 50 Carbon, 100 Silicates @OTHYONAL (4.2 ly)",
		"Dispatching F1 (4.5 ly)",
		"    collect OTHYONAL-3: 50 Carbon, 60 Silicates",
		"    collect OTHYONAL-BELT-1: 40 Silicates",
		"Q-1 requires 1 Carbon @S (0.0 ly)",
		"No idle freighter",
		"    collect S-1: 1 Carbon",
		"Z-1 ready to complete [NO BEACON]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("describe =\n%q\nwant\n%q", got, want)
	}
}
