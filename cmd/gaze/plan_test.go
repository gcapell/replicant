package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/model"
)

// star's entry point is where relay(entry[name]) puts its relay.
func star(name string, x float64) model.Star {
	return model.Star{Designation: name, EntryPoint: entry[name], Position: model.Position{X: x}}
}

var entry = map[string]string{"A": "A-3-L4", "B": "B-1-L4", "C": "C-2-L5", "D": "D-1-L4"}

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

func transport(code, loc string, capacity int) model.Device {
	return model.Device{DeviceCode: code, Location: loc, Status: "idle",
		Features: []string{"surge", "attach"}, AttachCapacity: capacity}
}

func observatory(code, loc, status string) model.Device {
	return model.Device{DeviceCode: code, DeviceType: "galactic_observatory", Location: loc, Status: status}
}

func TestCandidates(t *testing.T) {
	stars := []model.Star{star("A", 0), star("B", 10), star("C", 4), star("D", 100)}
	cands, centre, off, _ := candidates(stars, set("A", "B", "C", "Z"))
	if centre.X != 14.0/3 {
		t.Errorf("centre %v, want x=14/3", centre)
	}
	var names []string
	for _, c := range cands {
		names = append(names, c.Name)
	}
	if want := []string{"C", "A", "B"}; !slices.Equal(names, want) {
		t.Errorf("order %v, want %v", names, want)
	}
	if cands[2].Dest != "B-1-L4" || cands[2].Dir.X != 10-14.0/3 {
		t.Errorf("B = %+v", cands[2])
	}
	if !slices.Equal(off, []string{"Z"}) {
		t.Errorf("offMap %v, want [Z]", off)
	}
}

// A relaying star whose entry point isn't known is skipped.
func TestCandidatesNoEntryPoint(t *testing.T) {
	cands, _, _, missing := candidates(
		[]model.Star{star("A", 0), star("B", 10), star("C", 4), star("E", 3)},
		set("A", "B", "E"))
	var got []string
	for _, c := range cands {
		got = append(got, c.Name+" "+c.Dest)
	}
	if want := []string{"A A-3-L4", "B B-1-L4"}; !slices.Equal(got, want) {
		t.Errorf("candidates %v, want %v", got, want)
	}
	if !slices.Equal(missing, []string{"E"}) {
		t.Errorf("noEntry %v, want [E]", missing)
	}
}

// threeStars: candidates nearest-first are C (x=4), A (x=0), B (x=10). Stars
// F (x=1) and G (x=20) have no relay.
func threeStars() []candidate {
	cands, _, _, _ := candidates(
		[]model.Star{star("A", 0), star("B", 10), star("C", 4)},
		set("A", "B", "C"))
	return cands
}

var starPos = map[string]model.Position{
	"A": {X: 0}, "B": {X: 10}, "C": {X: 4}, "F": {X: 1}, "G": {X: 20},
}

func steps(p obsPlan) string {
	var ss []string
	for _, s := range p.Steps {
		ss = append(ss, s.String())
	}
	return strings.Join(ss, " | ")
}

func TestPlan(t *testing.T) {
	led := ledger{"A": ""}
	tests := []struct {
		name      string
		obs       []model.Device
		others    []model.Device
		target    string // for the first observatory
		transport string
		steps     []string // substrings of the first observatory's steps, in order
		problem   string
	}{
		{
			name:      "compacted at prospected star moves to nearest-centre star",
			obs:       []model.Device{observatory("O1", "A-3-L4", compacted)},
			others:    []model.Device{transport("T2", "A-3-L4", 4), transport("T1", "A-3-L4", 2), transport("T0", "B-1-L4", 1)},
			target:    "C",
			transport: "T1",
			steps:     []string{"attach to T1", "T1 travels A-3-L4 -> C-2-L5", "detach from T1", "unfurl"},
		},
		{
			name:    "compacted with no transport is stuck",
			obs:     []model.Device{observatory("O1", "A-3-L4", compacted)},
			others:  []model.Device{transport("T1", "A-3-L4", 1)},
			problem: "no free",
		},
		{
			name: "same system beats a nearer star",
			obs:  []model.Device{observatory("O1", "A-3-L4", compacted)},
			others: []model.Device{transport("TF", "F-1-L4", 1), transport("TA", "A-BELT-1", 9),
				transport("TG", "G-1", 1)},
			target:    "C",
			transport: "TA",
			steps:     []string{"fetch TA A-BELT-1 -> A-3-L4", "attach to TA", "TA travels A-3-L4 -> C-2-L5", "detach", "unfurl"},
		},
		{
			name:      "nearest star, then smallest capacity",
			obs:       []model.Device{observatory("O1", "A-3-L4", compacted)},
			others:    []model.Device{transport("TG", "G-1", 1), transport("TF2", "F-2", 4), transport("TF1", "F-1-L4", 2)},
			target:    "C",
			transport: "TF1",
			steps:     []string{"fetch TF1 F-1-L4 -> A-3-L4", "attach to TF1", "-> C-2-L5", "detach", "unfurl"},
		},
		{
			name: "busy, carrying or unknown-star transports are skipped",
			obs:  []model.Device{observatory("O1", "A-3-L4", compacted)},
			others: []model.Device{
				{DeviceCode: "T1", Location: "A-3-L4", Status: "travelling", Features: []string{"surge", "attach"}, AttachCapacity: 1},
				{DeviceCode: "T2", Location: "A-3-L4", Status: "idle", Features: []string{"surge", "attach"}, AttachCapacity: 4, AttachUsed: 1},
				{DeviceCode: "T3", Location: "A-3-L4", Status: "idle", Features: []string{"attach"}, AttachCapacity: 1},
				transport("T4", "Q-1", 1),
				transport("T5", "G-1", 9),
			},
			target:    "C",
			transport: "T5",
			steps:     []string{"fetch T5 G-1 -> A-3-L4", "attach to T5", "-> C-2-L5", "detach", "unfurl"},
		},
		{
			name:   "compacted at fresh star unfurls there",
			obs:    []model.Device{observatory("O1", "B-1-L4", compacted)},
			target: "B",
			steps:  []string{"unfurl"},
		},
		{
			name:   "idle at fresh star prospects outward",
			obs:    []model.Device{observatory("O1", "B-1-L4", idle)},
			target: "B",
			steps:  []string{"prospect from B direction [5.33, 0.00, 0.00]"},
		},
		{
			name:  "idle at prospected star compacts",
			obs:   []model.Device{observatory("O1", "A-3-L4", idle)},
			steps: []string{"compact"},
		},
		{
			name: "attached and arrived detaches",
			obs: []model.Device{{DeviceCode: "O1", Location: "C-2-L5", Status: compacted,
				AttachedToDeviceCode: "T1"}},
			others:    []model.Device{transport("T1", "C-2-L5", 1)},
			target:    "C",
			transport: "T1",
			steps:     []string{"detach from T1", "unfurl"},
		},
		{
			name: "observatory heading to C pushes the next one on to B",
			obs: []model.Device{
				{DeviceCode: "O2", Location: "A-3-L4", Status: compacted, AttachedToDeviceCode: "T9"},
				observatory("O1", "A-3-L4", compacted),
			},
			others: []model.Device{
				{DeviceCode: "T9", Status: "travelling", Travel: model.Travel{FinalDestination: "C-2-L5"}},
				transport("T1", "A-3-L4", 1),
			},
			target:    "B",
			transport: "T1",
			steps:     []string{"attach to T1", "-> B-1-L4", "detach from T1", "unfurl"},
		},
		{
			name: "two at the same fresh star: the second moves on",
			obs: []model.Device{
				observatory("O0", "C-2-L5", idle),
				observatory("O1", "C-2-L5", compacted),
			},
			others:    []model.Device{transport("T1", "C-2-L5", 1)},
			target:    "B",
			transport: "T1",
			steps:     []string{"attach to T1", "-> B-1-L4", "detach from T1", "unfurl"},
		},
		{
			name:    "unknown status",
			obs:     []model.Device{observatory("O1", "A-3-L4", "deactivated")},
			problem: "unknown status",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.problem == "no free" {
				tt.others[0].AttachUsed = 1
			}
			var p obsPlan
			for _, pl := range planAll(tt.obs, tt.others, threeStars(), starPos, led) {
				if pl.Obs.DeviceCode == "O1" {
					p = pl
				}
			}
			if tt.problem != "" {
				if !strings.Contains(p.Problem, tt.problem) {
					t.Errorf("problem %q, want %q", p.Problem, tt.problem)
				}
				return
			}
			if p.Problem != "" {
				t.Fatalf("unexpected problem %q", p.Problem)
			}
			got := ""
			if p.Target != nil {
				got = p.Target.Name
			}
			if got != tt.target {
				t.Errorf("target %q, want %q", got, tt.target)
			}
			if p.Transport != tt.transport {
				t.Errorf("transport %q, want %q", p.Transport, tt.transport)
			}
			if len(p.Steps) != len(tt.steps) {
				t.Fatalf("steps %q, want %d like %q", steps(p), len(tt.steps), tt.steps)
			}
			for i, s := range tt.steps {
				if !strings.Contains(p.Steps[i].String(), s) {
					t.Errorf("step %d %q, want it to contain %q", i, p.Steps[i], s)
				}
			}
		})
	}
}
