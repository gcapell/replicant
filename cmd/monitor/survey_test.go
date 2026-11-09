package main

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestPlanSurveys(t *testing.T) {
	pos := map[string]model.Position{
		"HOME": {X: 0}, "A": {X: 1}, "B": {X: 5}, "C": {X: 9}, "D": {X: 20}, "E": {X: 2},
	}
	existing := []store.Task{
		{ID: 1, Task: surveyTask, Location: "B", Status: "pending"},
		{ID: 2, Task: surveyTask, Location: "C", Status: "assigned", Assignee: "BUSY"},
		{ID: 3, Task: surveyTask, Location: "D", Status: "failed"},
	}
	teams := []store.SurveyTeam{
		{Controller: "T1", Star: "HOME", Drones: 5},
		{Controller: "T2", Star: "C", Drones: 6},
	}
	// A and E need new tasks; B is pending; C is assigned and D failed, so
	// neither is filed again or offered to a team.
	p, err := planSurveys([]string{"A", "B", "C", "D", "E"}, existing, teams, pos)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"A", "E"}; !slices.Equal(p.New, want) {
		t.Errorf("New = %v, want %v", p.New, want)
	}
	var got []string
	for _, a := range p.Assign {
		got = append(got, fmt.Sprintf("%s<-%s %.0f", a.Star, a.Team.Controller, a.Distance))
	}
	// T2 (at 9) takes B (at 5), T1 (at 0) the nearer of A and E: a longest
	// trip of 4. Giving T2 E instead would be 7.
	if want := []string{"A<-T1 1", "B<-T2 4"}; !slices.Equal(got, want) {
		t.Errorf("Assign = %v, want %v", got, want)
	}
	if want := []string{"E"}; !slices.Equal(p.Waiting, want) {
		t.Errorf("Waiting = %v, want %v", p.Waiting, want)
	}

	if _, err := planSurveys([]string{"NOWHERE"}, nil, teams, pos); err == nil {
		t.Error("no error for a star with no position")
	}
}

// surveyStore has relay stars NEAR (unsurveyed), FAR (unsurveyed) and DONE
// (fully surveyed), a free team at HOME, and controllers that don't qualify.
func surveyStore(t *testing.T) *sql.DB {
	t.Helper()
	db := openStore(t)
	ctx := t.Context()
	for _, s := range []model.Star{
		{Designation: "HOME", Position: model.Position{X: 0}},
		{Designation: "NEAR", Position: model.Position{X: 1}},
		{Designation: "FAR", Position: model.Position{X: 9}},
		{Designation: "DONE", Position: model.Position{X: 2}},
	} {
		if err := store.PutStar(ctx, db, s); err != nil {
			t.Fatal(err)
		}
	}
	exec(t, db, `UPDATE stars SET planet_count = 2, moon_count = 1, planets_scanned = 0, moons_scanned = 0 WHERE designation IN ('NEAR', 'FAR')`)
	exec(t, db, `UPDATE stars SET planet_count = 2, moon_count = 1, planets_scanned = 2, moons_scanned = 1 WHERE designation = 'DONE'`)
	devices := []model.Device{
		{DeviceCode: "RNEAR", DeviceType: "ftl_relay", Status: "relaying", Location: "NEAR-1-L4"},
		{DeviceCode: "RFAR", DeviceType: "ftl_relay", Status: "relaying", Location: "FAR-1-L4"},
		{DeviceCode: "RDONE", DeviceType: "ftl_relay", Status: "relaying", Location: "DONE-1-L4"},
		// HOME has a relay that isn't relaying: not part of the mesh.
		{DeviceCode: "RHOME", DeviceType: "ftl_relay", Status: "idle", Location: "HOME-BELT-1"},
		{DeviceCode: "GOOD", DeviceType: "ami_survey_controller", Status: "coordinating", Location: "HOME-1-L4"},
		{DeviceCode: "FEW", DeviceType: "ami_survey_controller", Status: "idle", Location: "HOME-1-L4"},
		{DeviceCode: "BELT", DeviceType: "ami_survey_controller", Status: "coordinating", Location: "HOME-BELT-1"},
		{DeviceCode: "MOVING", DeviceType: "ami_survey_controller", Status: "travelling"},
		{DeviceCode: "BS", DeviceType: "belt_surveyor", Status: "searching", Location: "HOME-BELT-1", ControllerDeviceCode: "BELT"},
	}
	add := func(controller string, n int) {
		for i := range n {
			devices = append(devices, model.Device{
				DeviceCode: fmt.Sprintf("%s-D%d", controller, i), DeviceType: "survey_drone",
				Status: "idle", Location: "HOME-1-L4", ControllerDeviceCode: controller,
			})
		}
	}
	add("GOOD", 5)
	add("FEW", 4)
	add("BELT", 5)
	add("MOVING", 5)
	for _, d := range devices {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func exec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query); err != nil {
		t.Fatal(err)
	}
}

func surveyTasks(t *testing.T, db *sql.DB) []string {
	t.Helper()
	ts, err := store.TasksOfType(t.Context(), db, surveyTask)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range ts {
		got = append(got, fmt.Sprintf("%s %s %s", x.Location, x.Status, x.Assignee))
	}
	return got
}

func TestFileSurveys(t *testing.T) {
	db := surveyStore(t)
	ctx := t.Context()
	var out strings.Builder
	if err := fileSurveys(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"FAR pending ", "NEAR assigned GOOD"}
	if got := surveyTasks(t, db); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q\noutput:\n%s", got, want, out.String())
	}

	// Again: nothing new to file, and GOOD is taken.
	out.Reset()
	if err := fileSurveys(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	if got := surveyTasks(t, db); !slices.Equal(got, want) {
		t.Errorf("second run: tasks = %q, want %q", got, want)
	}
	if !strings.HasPrefix(out.String(), "survey tasks: 2 already filed, 0 new, 0 to assign, 1 left pending") {
		t.Errorf("second run printed:\n%s", out.String())
	}
}

func TestFileSurveysUnreadSystem(t *testing.T) {
	db := surveyStore(t)
	exec(t, db, `UPDATE stars SET planet_count = NULL WHERE designation = 'FAR'`)
	err := fileSurveys(t.Context(), db, new(strings.Builder))
	if err == nil || !strings.Contains(err.Error(), "FAR") {
		t.Errorf("err = %v, want one about FAR", err)
	}
	if got := surveyTasks(t, db); len(got) != 0 {
		t.Errorf("tasks filed despite the error: %q", got)
	}
}

func TestDecideSurvey(t *testing.T) {
	at := func(code, loc string) member { return member{Code: code, Location: loc, Status: "idle"} }
	with := func(m member, f func(*member)) member { f(&m); return m }
	surveying := func(m *member) { m.Directive = surveyDirective }
	base := surveyState{Target: "FAR", TargetEntry: "FAR-2-L4", HomeEntry: "HOME-1-L4"}
	state := func(complete bool, team ...member) surveyState {
		s := base
		s.Complete, s.Team = complete, team
		return s
	}
	for _, tc := range []struct {
		name   string
		s      surveyState
		step   surveyStep
		who    []string
		dest   string
		errHas string
	}{
		{name: "travelling", s: state(false, at("C", ""), at("D1", "HOME-1-L4")), step: surveyWait},
		{name: "drone travelling", s: state(false, at("C", "FAR-3"), at("D1", "")), step: surveyWait},
		{name: "split", s: state(false, at("C", "FAR-3"), at("D1", "HOME-1")), errHas: "split"},
		{name: "other directive", s: state(false, with(at("C", "FAR-3"), func(m *member) { m.Directive = "patrol" }), at("D1", "FAR-3")), errHas: "patrol"},

		{name: "in target, start", s: state(false, at("C", "FAR-3"), at("D1", "FAR-4")), step: surveyStart},
		{name: "in target, surveying", s: state(false, with(at("C", "FAR-3"), surveying), at("D1", "FAR-4")), step: surveyWait},
		{name: "in target, complete but directive still on", s: state(true, with(at("C", "FAR-3"), surveying), at("D1", "FAR-4")), step: surveyWait},
		{name: "in target, attached", s: state(false, with(at("C", "FAR-2-L4"), func(m *member) { m.AttachedTo = "T" }), at("D1", "FAR-2-L4")), errHas: "attached"},
		{name: "in target, gather", s: state(true, at("C", "FAR-2-L4"), at("D1", "FAR-4"), at("D2", "FAR-2-L4")), step: surveyGather, who: []string{"D1"}, dest: "FAR-2-L4"},
		{name: "in target, done", s: state(true, at("C", "FAR-2-L4"), at("D1", "FAR-2-L4")), step: surveyDone},

		{name: "elsewhere, gather", s: state(false, at("C", "HOME-3"), at("D1", "HOME-1-L4")), step: surveyGather, who: []string{"C"}, dest: "HOME-1-L4"},
		{name: "elsewhere, at entry", s: state(false, at("C", "HOME-1-L4"), at("D1", "HOME-1-L4")), step: surveyBlock, dest: "HOME-1-L4"},
		{name: "elsewhere, at entry, attached", s: state(false, with(at("C", "HOME-1-L4"), func(m *member) { m.AttachedTo = "T" }), at("D1", "HOME-1-L4")), step: surveyBlock, dest: "HOME-1-L4"},
		{name: "elsewhere, attached away", s: state(false, with(at("C", "HOME-3"), func(m *member) { m.AttachedTo = "T" }), at("D1", "HOME-3")), errHas: "attached"},
		{name: "elsewhere, directive", s: state(false, with(at("C", "HOME-3"), surveying), at("D1", "HOME-3")), errHas: "running"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := decideSurvey(tc.s)
			if tc.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want one containing %q (action %+v)", err, tc.errHas, a)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if a.Step != tc.step || !slices.Equal(a.Travellers, tc.who) || a.Dest != tc.dest {
				t.Errorf("got %+v, want step %d %v %q", a, tc.step, tc.who, tc.dest)
			}
		})
	}
}
