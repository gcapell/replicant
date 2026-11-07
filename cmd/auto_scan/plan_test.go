package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/model"
)

func sys(name string, x, y float64, work int) system {
	return system{Name: name, Pos: model.Position{X: x, Y: y}, Planets: work, Unscanned: work}
}

func tm(id string, x, y float64) team {
	return team{Transport: id, Star: "S-" + id, Pos: model.Position{X: x, Y: y}}
}

func names(ss []system) []string {
	var n []string
	for _, s := range ss {
		n = append(n, s.Name)
	}
	return n
}

func TestGroupBalances(t *testing.T) {
	// Seeds at 0 and 100. Group 0's seed already has lots of work, so the
	// middle systems go to group 1 even though some are nearer 0.
	systems := []system{sys("L", 0, 0, 20), sys("R", 100, 0, 1),
		sys("M1", 40, 0, 1), sys("M2", 60, 0, 1), sys("M3", 45, 0, 1)}
	groups := group(systems, []int{0, 1})
	if got := names(groups[0]); !slices.Equal(got, []string{"L"}) {
		t.Errorf("group 0 = %v, want [L]", got)
	}
	if got := len(groups[1]); got != 4 {
		t.Errorf("group 1 has %d systems, want 4", got)
	}
}

func TestGroupNearestWhenBalanced(t *testing.T) {
	systems := []system{sys("L", 0, 0, 1), sys("R", 100, 0, 1),
		sys("L1", 5, 0, 1), sys("R1", 95, 0, 1)}
	groups := group(systems, []int{0, 1})
	if a, b := names(groups[0]), names(groups[1]); !slices.Equal(a, []string{"L", "L1"}) || !slices.Equal(b, []string{"R", "R1"}) {
		t.Errorf("groups = %v %v", a, b)
	}
}

func TestPlanMoreTeamsThanSystems(t *testing.T) {
	routes := plan([]team{tm("T1", 0, 0), tm("T2", 100, 0)}, []system{sys("X", 90, 0, 3)})
	if len(routes[0].Stops) != 0 || !slices.Equal(names(routes[1].Stops), []string{"X"}) {
		t.Errorf("routes = %+v", routes)
	}
	lines := describe(routes)
	if !strings.HasSuffix(lines[0], "nothing to do") || !strings.Contains(lines[1], "X (10.0ly, 3P/0M)") {
		t.Errorf("describe = %q", lines)
	}
}

func TestPlanCoversEverySystemOnce(t *testing.T) {
	var systems []system
	for i := range 20 {
		systems = append(systems, sys(string(rune('A'+i)), float64(i*7%23), float64(i*11%17), i%4+1))
	}
	routes := plan([]team{tm("T1", 0, 0), tm("T2", 20, 20), tm("T3", 0, 20)}, systems)
	var all []string
	for _, r := range routes {
		if len(r.Stops) == 0 {
			t.Errorf("team %s got nothing", r.Team.Transport)
		}
		all = append(all, names(r.Stops)...)
	}
	slices.Sort(all)
	if want := names(systems); !slices.Equal(all, want) {
		t.Errorf("visited %v, want %v", all, want)
	}
}

func transportDevice(attached ...model.AttachedDevice) *model.Device {
	return &model.Device{DeviceCode: "T", DeviceType: "transport", Status: "idle", Location: "MIRETH-4-L4",
		Features: []string{"cruise", "surge", "attach"}, AttachedDevices: attached}
}

func TestCheckTransport(t *testing.T) {
	ctl := model.AttachedDevice{DeviceCode: "C", DeviceType: "ami_survey_controller"}
	d := func(c string) model.AttachedDevice {
		return model.AttachedDevice{DeviceCode: c, DeviceType: "survey_drone"}
	}

	tm, err := checkTransport(transportDevice(ctl, d("1"), d("2"), d("3")))
	if err != nil || tm.Controller != "C" || len(tm.Drones) != 3 || tm.Star != "MIRETH" {
		t.Errorf("good team: %+v, %v", tm, err)
	}

	bad := map[string]*model.Device{
		"2 drones":      transportDevice(ctl, d("1"), d("2")),
		"no controller": transportDevice(d("1"), d("2"), d("3")),
		"2 controllers": transportDevice(ctl, ctl, d("1"), d("2"), d("3")),
		"extra":         transportDevice(ctl, d("1"), d("2"), d("3"), model.AttachedDevice{DeviceCode: "M", DeviceType: "miner"}),
	}
	busy := transportDevice(ctl, d("1"), d("2"), d("3"))
	busy.Status = "travelling"
	bad["travelling"] = busy
	noSurge := transportDevice(ctl, d("1"), d("2"), d("3"))
	noSurge.Features = []string{"attach"}
	bad["no surge"] = noSurge
	for name, dev := range bad {
		if _, err := checkTransport(dev); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
