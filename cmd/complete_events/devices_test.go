package main

import (
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

func at(loc string, x float64) place { return place{Loc: loc, Pos: model.Position{X: x}} }

// stopList lists a route as "attach CODE@LOC" / "detach CODE@LOC".
func stopList(r []stop, spares []spare) []string {
	var v []string
	for _, s := range r {
		verb := "detach "
		if s.Pickup {
			verb = "attach "
		}
		v = append(v, verb+spares[s.Spare].Code+"@"+s.At.Loc)
	}
	return v
}

func TestCost(t *testing.T) {
	if c := cost(at("A-1", 0), at("A-1", 0)); c != 0 {
		t.Errorf("same place = %v", c)
	}
	if c := cost(at("A-1", 0), at("A-BELT-1", 0)); c != inSystemCost {
		t.Errorf("same star = %v", c)
	}
	if c := cost(at("A-1", 0), at("B-1", 5)); c != 5+inSystemCost {
		t.Errorf("other star = %v", c)
	}
}

func TestPlanDevicesSingle(t *testing.T) {
	jobs := []job{{Type: "ftl_beacon", Dest: at("E-2", 10)}}
	spares := []spare{
		{Code: "FAR", Type: "ftl_beacon", At: at("F-BELT-1", 50)},
		{Code: "NEAR", Type: "ftl_beacon", At: at("N-BELT-1", 8)},
		{Code: "GRID", Type: "defence_grid", At: at("N-BELT-1", 8)},
	}
	carriers := []carrier{{Code: "C", At: at("N-3", 8), Free: 1}}
	dp := planDevices(jobs, spares, carriers)
	want := []string{"attach NEAR@N-BELT-1", "detach NEAR@E-2"}
	if got := stopList(dp.Routes[0], spares); !slices.Equal(got, want) || len(dp.Unassigned) != 0 {
		t.Errorf("route = %v, unassigned %v", got, dp.Unassigned)
	}
}

func TestPlanDevicesBatchesWithinCapacity(t *testing.T) {
	// Two beacons at one belt, both wanted at far-away E. A carrier with
	// room for 2 takes both in one trip; with room for 1 it must go twice.
	jobs := []job{{Type: "b", Dest: at("E-1", 100)}, {Type: "b", Dest: at("E-2", 100)}}
	spares := []spare{{Code: "B1", Type: "b", At: at("S-BELT-1", 0)}, {Code: "B2", Type: "b", At: at("S-BELT-1", 0)}}

	dp := planDevices(jobs, spares, []carrier{{Code: "C", At: at("S-BELT-1", 0), Free: 2}})
	if r := dp.Routes[0]; !r[0].Pickup || !r[1].Pickup || r[2].Pickup || r[3].Pickup {
		t.Errorf("free 2: %v", stopList(r, spares))
	}
	if got := routeCost(at("S-BELT-1", 0), dp.Routes[0]); got > 100.21 {
		t.Errorf("free 2 cost = %v", got)
	}

	dp = planDevices(jobs, spares, []carrier{{Code: "C", At: at("S-BELT-1", 0), Free: 1}})
	if !fits(dp.Routes[0], 1) || len(dp.Routes[0]) != 4 {
		t.Errorf("free 1: %v", stopList(dp.Routes[0], spares))
	}
}

func TestPlanDevicesSpreadsForFinishTime(t *testing.T) {
	// Jobs east and west, one carrier at each end: each should take its own
	// side rather than one carrier doing both.
	jobs := []job{{Type: "b", Dest: at("W-1", -100)}, {Type: "b", Dest: at("E-1", 100)}}
	spares := []spare{{Code: "BW", Type: "b", At: at("W-BELT-1", -90)}, {Code: "BE", Type: "b", At: at("E-BELT-1", 90)}}
	carriers := []carrier{{Code: "CW", At: at("W-2", -80), Free: 9}, {Code: "CE", At: at("E-2", 80), Free: 9}}
	dp := planDevices(jobs, spares, carriers)
	if got := stopList(dp.Routes[0], spares); !slices.Equal(got, []string{"attach BW@W-BELT-1", "detach BW@W-1"}) {
		t.Errorf("CW = %v", got)
	}
	if got := stopList(dp.Routes[1], spares); !slices.Equal(got, []string{"attach BE@E-BELT-1", "detach BE@E-1"}) {
		t.Errorf("CE = %v", got)
	}
}

func TestPlanDevicesUnassigned(t *testing.T) {
	jobs := []job{{Type: "x", Dest: at("E-1", 0)}, {Type: "b", Dest: at("E-1", 0)}}
	spares := []spare{{Code: "B", Type: "b", At: at("S-BELT-1", 0)}}
	dp := planDevices(jobs, spares, []carrier{{Code: "C", At: at("S-1", 0), Free: 0}})
	if dp.Unassigned[0] != "no spare x" || dp.Unassigned[1] != "no carrier with room" {
		t.Errorf("unassigned = %v", dp.Unassigned)
	}
}

func TestPlanDevicesEachSpareOnce(t *testing.T) {
	jobs := []job{{Type: "b", Dest: at("E-1", 0)}, {Type: "b", Dest: at("E-2", 0)}}
	spares := []spare{{Code: "B", Type: "b", At: at("S-BELT-1", 0)}}
	dp := planDevices(jobs, spares, []carrier{{Code: "C", At: at("S-1", 0), Free: 9}})
	if len(dp.Routes[0]) != 2 || len(dp.Unassigned) != 1 {
		t.Errorf("routes %v, unassigned %v", stopList(dp.Routes[0], spares), dp.Unassigned)
	}
}

func TestChooseOption(t *testing.T) {
	dev := func(typ string, req int) model.RequiredDevice {
		return model.RequiredDevice{DeviceType: typ, Required: req}
	}
	ev := &model.LocationEvent{Progress: model.EventProgress{Options: []model.EventOption{
		{Devices: []model.RequiredDevice{dev("grid", 1)}},
		{Devices: []model.RequiredDevice{dev("probe", 2)}},
	}}}
	spare := map[string]int{"probe": 3}
	if got := chooseOption(ev, spare); got != 1 || spare["probe"] != 1 {
		t.Errorf("choice %d, spare %v", got, spare)
	}
	// Now only one probe is left: neither option is possible, so the first.
	if got := chooseOption(ev, spare); got != 0 {
		t.Errorf("second choice %d", got)
	}
	// Option 2's probes have arrived (met stays false until a replicant is
	// there): it's satisfied, so it's chosen.
	ev.Progress.Options[1].Devices[0].Current = 2
	if got := chooseOption(ev, spare); got != 1 {
		t.Errorf("satisfied choice %d", got)
	}
}

func TestIsSpare(t *testing.T) {
	ev := map[string]bool{"E-2": true}
	for _, tc := range []struct {
		d    model.Device
		want bool
	}{
		{model.Device{DeviceType: "ftl_beacon", Location: "S-BELT-1", Status: "inactive"}, true},
		{model.Device{DeviceType: "ftl_beacon", Location: "S-3", Status: "idle"}, false},
		{model.Device{DeviceType: "ftl_beacon", Location: "S-BELT-1", AttachedToDeviceCode: "C"}, false},
		{model.Device{DeviceType: "grid", Location: "S-3", Status: "idle"}, true},
		{model.Device{DeviceType: "grid", Location: "S-3", Status: "active"}, false},
		{model.Device{DeviceType: "grid", Location: "E-2", Status: "idle"}, false},
		{model.Device{DeviceType: "grid", Location: "S-OBJ-9", Status: "idle"}, false},
		{model.Device{DeviceType: "grid", Location: "S-3", Status: "idle", StowedInDeviceCode: "V"}, false},
	} {
		if got := isSpare(tc.d, ev); got != tc.want {
			t.Errorf("isSpare(%+v) = %v", tc.d, got)
		}
	}
}

func TestDeviceJobs(t *testing.T) {
	needs := []need{
		{Loc: "B-1", Beacon: true, Devices: []model.RequiredDevice{{DeviceType: "grid", Current: 1, Required: 3}}},
		{Loc: "A-1"},
	}
	var got []string
	for _, j := range deviceJobs(needs) {
		got = append(got, j.Type+"@"+j.Dest.Loc)
	}
	want := []string{"ftl_beacon@A-1", "grid@B-1", "grid@B-1"}
	if !slices.Equal(got, want) {
		t.Errorf("jobs = %v", got)
	}
}
