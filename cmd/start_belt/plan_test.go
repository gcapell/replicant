package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

// beltOrder is the order of belt's level arguments.
var beltOrder = []model.ResourceType{
	model.Volatiles, model.Rares,
	model.Carbon, model.Conductive, model.Silicates, model.Structural,
}

func belt(name, density string, lv ...string) Belt {
	l := map[model.ResourceType]string{}
	for i, r := range beltOrder {
		l[r] = "low"
		if i < len(lv) {
			l[r] = lv[i]
		}
	}
	return Belt{Designation: name + "-BELT-1", Star: name, Density: density, Levels: l}
}

func names(bs []Belt) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Designation)
	}
	return out
}

// team returns a complete free team at loc, with codes prefixed by p.
func team(p, loc string) []Device {
	star := loc[:3]
	d := func(code, typ string) Device {
		return Device{Code: p + code, Type: typ, Status: "idle", Location: loc, Star: star}
	}
	ds := []Device{
		d("MC", "ami_mining_controller"), d("SC", "ami_survey_controller"),
		d("BS", "belt_surveyor"), d("MD", "maintenance_drone"),
	}
	for i := range 6 {
		ds = append(ds, d(fmt.Sprint("M", i), "mining_drone"))
	}
	return ds
}

func TestScoreOrder(t *testing.T) {
	bs := []Belt{
		belt("A", "moderate", "rich", "rich"),
		belt("B", "dense", "low", "low", "rich"),         // a rich non-preferred
		belt("C", "dense", "rich"),                       // a rich preferred
		belt("D", "dense", "low", "low", "high", "high"), // no rich
		belt("E", "dense", "low", "low", "rich", "high"), // like B, plus a high
	}
	pos := map[string]model.Position{}
	for _, b := range bs {
		pos[b.Star] = b.Pos
	}
	pos["TMA"] = model.Position{}
	relays := map[string]bool{"A": true, "B": true, "C": true, "D": true, "E": true}
	devs := append(team("T", "TMA-BELT-1"), Device{Code: "SUR", Type: "surge_carrier", Status: "idle", Location: "TMA-1", Star: "TMA"})
	p, err := makePlan(bs, devs, relays, pos)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"C-BELT-1", "E-BELT-1", "B-BELT-1", "D-BELT-1", "A-BELT-1"}
	if got := names(p.Belts); !slices.Equal(got, want) {
		t.Errorf("order %v, want %v", got, want)
	}
}

func TestCandidates(t *testing.T) {
	bs := []Belt{belt("A", "dense"), belt("B", "dense"), belt("C", "dense"), belt("D", "dense")}
	devs := []Device{
		{Code: "1", Type: "ami_mining_controller", Status: "coordinating", Location: "A-BELT-1", Star: "A"},
		{Code: "2", Type: "ami_mining_controller", Status: "travelling", Destination: "B-BELT-1"},
	}
	relays := map[string]bool{"A": true, "B": true, "C": true}
	if got := names(candidates(bs, devs, relays)); !slices.Equal(got, []string{"C-BELT-1"}) {
		t.Errorf("candidates %v, want [C-BELT-1]", got)
	}
}

func TestTeams(t *testing.T) {
	ok := team("A", "AAA-BELT-1")
	// Controlled or attached drones don't count: leaves 4 free miners.
	short := team("B", "BBB-BELT-1")
	short[4].Control, short[5].AttachedTo = "X", "Y"
	short[6].Status = "mining (carbon)"
	// No free belt_surveyor.
	noSurveyor := team("C", "CCC-BELT-1")
	noSurveyor[2].Control = "X"

	ts := teams(slices.Concat(ok, short, noSurveyor))
	if len(ts) != 1 || ts[0].Location != "AAA-BELT-1" {
		t.Fatalf("teams %+v, want just AAA-BELT-1", ts)
	}
	if got := ts[0].Devices(); len(got) != 9 || !slices.Equal(ts[0].Miners, []string{"AM0", "AM1", "AM2", "AM3", "AM4"}) {
		t.Errorf("team devices %v", got)
	}
}

func TestNearestTeamAndCarrier(t *testing.T) {
	bs := []Belt{belt("ZZZ", "dense")}
	bs[0].Pos = model.Position{X: 10}
	pos := map[string]model.Position{
		"ZZZ": {X: 10}, "AAA": {X: 0}, "BBB": {X: 8}, "CCC": {X: 7}, "DDD": {X: 30},
	}
	devs := slices.Concat(team("A", "AAA-BELT-1"), team("B", "BBB-BELT-1"),
		[]Device{
			{Code: "C1", Type: "surge_carrier", Status: "idle", Location: "CCC-1", Star: "CCC"},
			{Code: "C2", Type: "surge_carrier", Status: "idle", Location: "BBB-2", Star: "BBB"},
			{Code: "C3", Type: "surge_carrier", Status: "idle", Location: "BBB-BELT-1", Star: "BBB"},
			{Code: "C4", Type: "surge_carrier", Status: "travelling", Destination: "BBB-BELT-1"},
			{Code: "C5", Type: "surge_platform", Status: "idle", Location: "BBB-BELT-1", Star: "BBB"},
		})
	p, err := makePlan(bs, devs, map[string]bool{"ZZZ": true}, pos)
	if err != nil {
		t.Fatal(err)
	}
	if p.Team.Location != "BBB-BELT-1" || p.Dist != 2 {
		t.Errorf("team at %s (%.1f ly), want BBB-BELT-1 (2)", p.Team.Location, p.Dist)
	}
	if p.Carrier.Code != "C3" {
		t.Errorf("carrier %+v, want C3", p.Carrier)
	}
}

func TestBadLevels(t *testing.T) {
	if _, err := score(belt("A", "thick")); err == nil {
		t.Error("accepted an unknown density")
	}
	if _, err := score(belt("A", "dense", "lots")); err == nil {
		t.Error("accepted an unknown level")
	}
}
