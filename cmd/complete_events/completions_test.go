package main

import (
	"slices"
	"testing"
)

func comp(loc string, x float64) completion { return completion{Event: loc + "-EVT-1", At: at(loc, x)} }

func stopLocs(r vesselRoute) []string {
	var l []string
	for _, s := range r.Stops {
		l = append(l, s.At.Loc)
	}
	return l
}

func TestReadyEvents(t *testing.T) {
	needs := []need{
		{Loc: "B-1", Event: "B-1-EVT-1", Met: true, Beacon: true},
		{Loc: "A-1", Met: true},    // no beacon
		{Loc: "C-1", Beacon: true}, // not satisfied
		{Loc: "D-1", Event: "D-1-EVT-2", Met: true, Beacon: true},
	}
	var got []string
	for _, c := range readyEvents(needs) {
		got = append(got, c.Event)
	}
	if !slices.Equal(got, []string{"B-1-EVT-1", "D-1-EVT-2"}) {
		t.Errorf("ready = %v", got)
	}
}

func TestPlanCompletions(t *testing.T) {
	// Two clusters, one vessel near each (listed the "wrong" way round).
	events := []completion{comp("L1", 0), comp("R1", 100), comp("L2", 10), comp("R2", 90), comp("L3", 5)}
	vessels := []vessel{{Code: "VR", At: at("VR-1", 120)}, {Code: "VL", At: at("VL-1", -20)}}
	routes := planCompletions(vessels, events)
	if got := stopLocs(routes[0]); !slices.Equal(got, []string{"R1", "R2"}) {
		t.Errorf("VR = %v", got)
	}
	if got := stopLocs(routes[1]); !slices.Equal(got, []string{"L1", "L3", "L2"}) {
		t.Errorf("VL = %v", got)
	}
}

func TestPlanCompletionsMoreVessels(t *testing.T) {
	vessels := []vessel{{Code: "FAR", At: at("F-1", 500)}, {Code: "NEAR", At: at("N-1", 1)}}
	routes := planCompletions(vessels, []completion{comp("E", 0)})
	if len(routes[0].Stops) != 0 || !slices.Equal(stopLocs(routes[1]), []string{"E"}) {
		t.Errorf("routes = %+v", routes)
	}
	if r := planCompletions(nil, []completion{comp("E", 0)}); len(r) != 0 {
		t.Errorf("no vessels: %+v", r)
	}
}

func TestDescribeCompletions(t *testing.T) {
	routes := []vesselRoute{
		{Vessel: vessel{Code: "V", At: at("S-1", 0)}, Stops: []completion{comp("A-2", 3), comp("B-4", 7)}},
		{Vessel: vessel{Code: "IDLE", At: at("S-1", 0)}},
	}
	got := describeCompletions(routes, []completion{comp("A-2", 3), comp("B-4", 7)})
	want := []string{"Completions: 2 ready events", "V from S-1: A-2 (3.0 ly) → B-4 (4.0 ly)"}
	if !slices.Equal(got, want) {
		t.Errorf("describe = %q", got)
	}
	if got := describeCompletions(nil, []completion{comp("A-2", 3)}); len(got) != 2 {
		t.Errorf("no vessels = %q", got)
	}
}
