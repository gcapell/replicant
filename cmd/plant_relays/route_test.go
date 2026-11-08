package main

// Run with a dummy key, e.g. API_KEY=x go test ./cmd/plant_relays (api's init exits without one).

import (
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

func at(name string, x, y float64) star {
	return star{Name: name, Pos: model.Position{X: x, Y: y}, Explored: true}
}

func xy(x, y float64) model.Position { return model.Position{X: x, Y: y} }

func names(route []star) []string {
	var out []string
	for _, s := range route {
		out = append(out, s.Name)
	}
	return out
}

func TestConnectableChainsOutward(t *testing.T) {
	// B is only in reach via A; FAR is out of reach of everything.
	mesh := []star{at("R", 0, 0)}
	cands := []star{at("B", 12, 0), at("FAR", 40, 0), at("A", 6, 0)}
	if got, want := names(connectable(mesh, cands)), []string{"A", "B"}; !slices.Equal(got, want) {
		t.Errorf("connectable = %v, want %v", got, want)
	}
}

func TestConnectableGrowsNearestFirst(t *testing.T) {
	mesh := []star{at("R", 0, 0)}
	cands := []star{at("A", 5, 0), at("B", -1, 0), at("C", 9, 0)}
	// B (1 from R), then A (5 from R), then C (4 from A).
	if got, want := names(connectable(mesh, cands)), []string{"B", "A", "C"}; !slices.Equal(got, want) {
		t.Errorf("connectable = %v, want %v", got, want)
	}
}

func TestReachIsStrict(t *testing.T) {
	if got := connectable([]star{at("R", 0, 0)}, []star{at("EDGE", reach, 0)}); len(got) != 0 {
		t.Errorf("got %v, want nothing at exactly %v ly", names(got), reach)
	}
}

func TestDescribe(t *testing.T) {
	mesh := []star{at("R", 0, 0), at("A", 6, 0), at("B", 12, 0)}
	stops := describe(xy(0, 0), 3, []star{at("A", 6, 0), at("B", 12, 0)}, mesh)
	if stops[1].Via != "A" || stops[1].ViaDist != 6 || stops[1].Total != 15 {
		t.Errorf("stop 2 = %+v", stops[1])
	}
}

func TestSplit(t *testing.T) {
	stars := []model.Star{{Designation: "R"}, {Designation: "A"}}
	in := split(stars, map[string]bool{"R": true, "ELSEWHERE": true})
	if len(in.mesh) != 1 || len(in.candidates) != 1 || !slices.Equal(in.offMap, []string{"ELSEWHERE"}) {
		t.Errorf("split = %+v", in)
	}
}

func TestRelayingStars(t *testing.T) {
	got := relayingStars([]model.Device{
		{Location: "MIRETH-4-L4", Status: "relaying"},
		{Location: "SOL-3-L5", Status: "relaying", StowedInDeviceCode: "V1"},
		{Location: "ZETA-BELT-1", Status: "idle"},
		{Location: "OMEGA-2-L4", Status: "inactive"},
	})
	if len(got) != 1 || !got["MIRETH"] {
		t.Errorf("relayingStars = %v", got)
	}
}

func TestAvailableBelts(t *testing.T) {
	got := availableBelts([]model.Device{
		{DeviceCode: "R2", Location: "ZETA-BELT-1", Status: "idle", ReplicantCode: "X"},
		{DeviceCode: "R1", Location: "ZETA-BELT-1", Status: "idle", ReplicantCode: "Y"},
		{DeviceCode: "R3", Location: "ALPHA-BELT-2", Status: "idle"},
		{DeviceCode: "S", Location: "ALPHA-BELT-2", Status: "idle", StowedInDeviceCode: "V"},
		{DeviceCode: "L", Location: "MIRETH-4-L4", Status: "relaying"},
		{DeviceCode: "P", Location: "MIRETH-4", Status: "idle"},
	})
	if len(got) != 2 || got[0].Loc != "ALPHA-BELT-2" || len(got[0].Relays) != 1 ||
		got[1].Loc != "ZETA-BELT-1" || !slices.Equal(got[1].Relays, []relay{{"R1", "Y"}, {"R2", "X"}}) {
		t.Errorf("availableBelts = %+v", got)
	}
}

func TestStowedRelayCodes(t *testing.T) {
	v := &model.Device{StowedDevices: []model.StowedDevice{
		{DeviceCode: "R1", DeviceType: "ftl_relay"},
		{DeviceCode: "M1", DeviceType: "replicant_matrix"},
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
