package main

import (
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

func at(name string, x, y float64) star {
	return star{Name: name, Pos: model.Position{X: x, Y: y}, Explored: true}
}

func names(route []star) []string {
	var out []string
	for _, s := range route {
		out = append(out, s.Name)
	}
	return out
}

func TestPlanChainsOutward(t *testing.T) {
	// B is out of reach of R but within reach of A, so A must come first,
	// even though the vessel starts nearer B.
	relays := []star{at("R", 0, 0)}
	cands := []star{at("B", 12, 0), at("A", 6, 0), at("FAR", 40, 0)}
	route := plan(model.Position{X: 13}, relays, cands, 10)
	if got, want := names(route), []string{"A", "B"}; !slices.Equal(got, want) {
		t.Errorf("route = %v, want %v", got, want)
	}
	if !valid(route, relays) {
		t.Error("route not valid")
	}
	if got := unreachable(route, cands); len(got) != 1 || got[0].Name != "FAR" {
		t.Errorf("unreachable = %v, want [FAR]", names(got))
	}
}

func TestPlanRespectsLimit(t *testing.T) {
	relays := []star{at("R", 0, 0)}
	cands := []star{at("A", 1, 0), at("B", 2, 0), at("C", 3, 0)}
	if got := plan(model.Position{}, relays, cands, 2); len(got) != 2 {
		t.Errorf("got %v, want 2 stops", names(got))
	}
}

func TestReachIsStrict(t *testing.T) {
	relays := []star{at("R", 0, 0)}
	if got := plan(model.Position{}, relays, []star{at("EDGE", reach, 0)}, 5); len(got) != 0 {
		t.Errorf("got %v, want nothing at exactly %v ly", names(got), reach)
	}
}

func TestTwoOptImproves(t *testing.T) {
	// A zig-zagging route that 2-opt should straighten out.
	relays := []star{at("R", 0, 0)}
	start := model.Position{}
	route := []star{at("A", 1, 0), at("B", -1, 0), at("C", 2, 0), at("D", -2, 0)}
	before := length(start, route)
	got := twoOpt(start, slices.Clone(route), relays)
	if after := length(start, got); after >= before {
		t.Errorf("2-opt didn't improve: %v (%.2f) -> %v (%.2f)", names(route), before, names(got), after)
	}
	if !valid(got, relays) {
		t.Error("2-opt produced invalid route")
	}
}

func TestTwoOptKeepsValidity(t *testing.T) {
	// Reversing A,B would be shorter from the start at 13, but B needs A.
	relays := []star{at("R", 0, 0)}
	got := twoOpt(model.Position{X: 13}, []star{at("A", 6, 0), at("B", 12, 0)}, relays)
	if want := []string{"A", "B"}; !slices.Equal(names(got), want) {
		t.Errorf("got %v, want %v", names(got), want)
	}
}

func TestPlanInTwo(t *testing.T) {
	relays := []star{at("R", 0, 0)}
	cands := []star{at("A", 2, 0), at("B", 4, 0), at("C", 9, 0), at("D", -3, 0)}
	route := planInTwo(model.Position{}, relays, cands, 2)
	if len(route) != 4 || !valid(route, relays) {
		t.Fatalf("route = %v", names(route))
	}
	// The first two are the best two-stop route on their own.
	if got := names(route[:2]); !slices.Equal(got, names(plan(model.Position{}, relays, cands, 2))) {
		t.Errorf("first part = %v", got)
	}
}

func TestDescribe(t *testing.T) {
	relays := []star{at("R", 0, 0)}
	stops := describe(model.Position{}, []star{at("A", 6, 0), at("B", 12, 0)}, relays)
	if stops[1].Via != "A" || stops[1].ViaDist != 6 || stops[1].Total != 12 {
		t.Errorf("stop 2 = %+v", stops[1])
	}
}

func TestSplit(t *testing.T) {
	stars := []model.Star{{Designation: "R"}, {Designation: "A"}}
	in := split(stars, map[string]bool{"R": true, "ELSEWHERE": true})
	if len(in.relays) != 1 || len(in.candidates) != 1 || !slices.Equal(in.offMap, []string{"ELSEWHERE"}) {
		t.Errorf("split = %+v", in)
	}
}

func TestRelayStarsSkipsStowed(t *testing.T) {
	got := relayStars([]model.Device{
		{Location: "MIRETH-4-L4"},
		{Location: "SOL-3-L5", StowedInDeviceCode: "V1"},
		{Location: ""},
	})
	if len(got) != 1 || !got["MIRETH"] {
		t.Errorf("relayStars = %v", got)
	}
}

func TestStowedRelayCodes(t *testing.T) {
	v := &model.Device{StowedDevices: []model.StowedDevice{
		{DeviceCode: "R1", DeviceType: "ftl_relay"},
		{DeviceCode: "M1", DeviceType: "mining_drone"},
		{DeviceCode: "R2", DeviceType: "ftl_relay"},
	}}
	if got := stowedRelayCodes(v); !slices.Equal(got, []string{"R1", "R2"}) {
		t.Errorf("stowedRelayCodes = %v", got)
	}
}

func TestToStarKeepsEntryPoint(t *testing.T) {
	if got := toStar(model.Star{Designation: "CORBRENEUS", EntryPoint: "CORBRENEUS-5-L4"}); got.EntryPoint != "CORBRENEUS-5-L4" {
		t.Errorf("toStar = %+v", got)
	}
	if got := oort("CORBRENEUS"); got != "CORBRENEUS-OORT" {
		t.Errorf("oort = %q", got)
	}
}
