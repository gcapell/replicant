package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestFormatResources(t *testing.T) {
	got, err := formatResources(map[model.ResourceType]int{"volatiles": 43, "carbon": 54, "structural": 109, "rares": 0})
	if err != nil || got != "C=54 S=109 V=43" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := formatResources(map[model.ResourceType]int{"unobtainium": 1}); err == nil {
		t.Error("unknown resource: no error")
	}
}

func discovered(designation, loc string, rs map[model.ResourceType]int) (api.Envelope, *model.SalvageDiscovered) {
	return api.Envelope{Event: "salvage.discovered", DeviceCode: "S", Star: "FAR", Location: "FAR-1-L4"},
		&model.SalvageDiscovered{Designation: designation, Location: loc, Resources: rs}
}

func TestSalvageDiscovered(t *testing.T) {
	db := taskStore(t)
	ctx := t.Context()
	apply := func(env api.Envelope, p *model.SalvageDiscovered) error {
		return inTx(ctx, db, func(tx *sql.Tx) error { return salvageDiscovered(ctx, tx, env, p) })
	}
	for _, e := range []struct {
		designation, loc string
		rs               map[model.ResourceType]int
	}{
		{"FAR-2-4-SAL-1", "FAR-2-4", map[model.ResourceType]int{"carbon": 54, "volatiles": 43}},
		{"FAR-2-4-SAL-1", "FAR-2-4", map[model.ResourceType]int{"carbon": 54, "volatiles": 43}}, // repeated
		{"FAR-2-4-SAL-2", "FAR-2-4", map[model.ResourceType]int{"rares": 333}},
		{"FAR-3-SAL-1", "FAR-3", map[model.ResourceType]int{"conductive": 43}},
	} {
		if err := apply(discovered(e.designation, e.loc, e.rs)); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		`1 salvage FAR-2-4 pending  {"sites":{"FAR-2-4-SAL-1":"C=54 V=43","FAR-2-4-SAL-2":"R=333"}}`,
		`2 salvage FAR-3 pending  {"sites":{"FAR-3-SAL-1":"Co=43"}}`,
	}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
	var pri int
	if err := db.QueryRowContext(ctx, `SELECT priority FROM tasks WHERE id = 1`).Scan(&pri); err != nil || pri != salvagePriority {
		t.Errorf("priority %d, %v; want %d", pri, err, salvagePriority)
	}

	for _, bad := range []struct{ designation, loc string }{
		{"FAR-1-L4-SAL-1", "FAR-1-L4"}, // not a planet or moon
		{"NEAR-2-SAL-1", "NEAR-2"},     // another star
		{"FAR-2-5-SAL-1", "FAR-2-4"},   // designation elsewhere
		{"FAR-BELT-1-SAL-1", "FAR-BELT-1"},
	} {
		if err := apply(discovered(bad.designation, bad.loc, nil)); err == nil {
			t.Errorf("%s at %s: no error", bad.designation, bad.loc)
		}
	}
}

func TestChooseSalvageSites(t *testing.T) {
	locs := map[string]model.LocationSummary{
		"FAR-2":      {ResourceSites: 1},
		"FAR-2-4":    {ResourceSites: 2},
		"FAR-3":      {ResourceSites: 0},
		"FAR-BELT-1": {ResourceSites: 3},
		"FAR-OBJ-X":  {ResourceSites: 1},
		"FAR-1-L4":   {ResourceSites: 1},
		"SOL-3":      {ResourceSites: 1},
		"NEAR-1":     {ResourceSites: 1},
	}
	sites, unrelayed := chooseSalvageSites(locs, []string{"FAR", "SOL"})
	if !slices.Equal(sites, []string{"FAR-2", "FAR-2-4"}) || !slices.Equal(unrelayed, []string{"NEAR-1"}) {
		t.Errorf("sites %v, unrelayed %v", sites, unrelayed)
	}
}

func TestFindSalvageSites(t *testing.T) {
	db := taskStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO devices (device_code, device_type, status, location) VALUES ('R', 'ftl_relay', 'relaying', 'FAR-1-L4')`)
	exec(t, db, `INSERT INTO tasks (task, location, status, created_at, updated_at) VALUES ('salvage', 'FAR-2', 'failed', 0, 0)`)
	saved := listLocations
	t.Cleanup(func() { listLocations = saved })
	listLocations = func(ctx context.Context) (*model.LocationList, error) {
		return &model.LocationList{Locations: map[string]model.LocationSummary{
			"FAR-2": {ResourceSites: 1}, "FAR-3": {ResourceSites: 1}, "NEAR-1": {ResourceSites: 1},
		}}, nil
	}
	var out strings.Builder
	if err := findSalvageSites(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"1 salvage FAR-2 failed", "2 salvage FAR-3 pending"}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q\n%s", got, want, out.String())
	}
	if !strings.Contains(out.String(), "NEAR-1") {
		t.Errorf("output doesn't list NEAR-1:\n%s", out.String())
	}
}

func TestSalvageTeamProblem(t *testing.T) {
	good := store.MiningTeam{Controller: "C", Location: "HOME-1", Star: "HOME", Miners: 5, Maint: "M", MaintLocation: "HOME-2", MaintStar: "HOME"}
	with := func(f func(*store.MiningTeam)) store.MiningTeam { t := good; f(&t); return t }
	for _, tc := range []struct {
		name    string
		team    store.MiningTeam
		problem string // substring; "" for none
	}{
		{"good", good, ""},
		{"exactly fits", with(func(t *store.MiningTeam) { t.Miners = 7 }), ""},
		{"too big", with(func(t *store.MiningTeam) { t.Miners = 8 }), "largest transport"},
		{"few miners", with(func(t *store.MiningTeam) { t.Miners = 3 }), "under 4"},
		{"no maint", with(func(t *store.MiningTeam) { t.Maint = "" }), "no maintenance_drone"},
		{"maint elsewhere", with(func(t *store.MiningTeam) { t.MaintStar, t.MaintLocation = "FAR", "FAR-1" }), "M@FAR-1 isn't in HOME"},
		{"travelling", with(func(t *store.MiningTeam) { t.Location, t.Star = "", "" }), "no location"},
	} {
		got, err := salvageTeamProblem(tc.team, 9)
		if err != nil || (tc.problem == "") != (got == "") || !strings.Contains(got, tc.problem) {
			t.Errorf("%s: problem %q, %v; want %q", tc.name, got, err, tc.problem)
		}
	}
	if _, err := salvageTeamProblem(with(func(t *store.MiningTeam) { t.Others = []string{"X@HOME-1 (belt_surveyor)"} }), 9); err == nil {
		t.Error("controlling a belt_surveyor: no error")
	}
}

// salvageStore has teams at HOME (GOOD, with 4 miners and a maintenance
// drone; NOMAINT, without), BUSY (assigned a task) and a surge_carrier.
func salvageStore(t *testing.T) *sql.DB {
	t.Helper()
	db := taskStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO blueprints (device_type, short_description, description, features, print_time,
		attach_capacity, cargo_capacity, stow_capacity, queue_size, relay_range, strength)
		VALUES ('surge_carrier', '', '', 'surge,cruise,attach', 1, 9, 0, 0, 0, 0, 0)`)
	devices := []model.Device{
		{DeviceCode: "T", DeviceType: "surge_carrier", Features: []string{"surge", "cruise", "attach"}, Status: "idle", Location: "HOME-1-L4"},
		{DeviceCode: "GOOD", DeviceType: "ami_mining_controller", Status: "coordinating", Location: "HOME-1"},
		{DeviceCode: "NOMAINT", DeviceType: "ami_mining_controller", Status: "idle", Location: "HOME-1"},
		{DeviceCode: "BUSY", DeviceType: "ami_mining_controller", Status: "coordinating", Location: "HOME-BELT-1"},
		{DeviceCode: "M", DeviceType: "maintenance_drone", Status: "idle", Location: "HOME-2"},
	}
	for _, c := range []string{"GOOD", "NOMAINT", "BUSY"} {
		for i := range 4 {
			devices = append(devices, model.Device{DeviceCode: fmt.Sprintf("%s-D%d", c, i), DeviceType: "mining_drone",
				Status: "idle", Location: "HOME-1", ControllerDeviceCode: c})
		}
	}
	for _, d := range devices {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	exec(t, db, `INSERT INTO adopted_maintenance_drones (drone, controller) VALUES ('M', 'GOOD')`)
	exec(t, db, `INSERT INTO tasks (task, location, status, assignee, created_at, updated_at) VALUES ('mine', 'HOME-BELT-1', 'assigned', 'BUSY', 0, 0)`)
	return db
}

func TestAssignSalvage(t *testing.T) {
	db := salvageStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO tasks (task, location, priority, created_at, updated_at) VALUES
		('salvage', 'FAR-2', 4, 0, 0), ('salvage', 'NEAR-3', 4, 0, 0)`)
	var out strings.Builder
	ids, err := assignSalvageDB(ctx, db, &out)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []int64{3}) {
		t.Errorf("assigned %v, want [3] (NEAR, the nearer)\n%s", ids, out.String())
	}
	want := []string{"1 mine HOME-BELT-1 assigned BUSY", "2 salvage FAR-2 pending", "3 salvage NEAR-3 assigned GOOD"}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
	if !strings.Contains(out.String(), "WARNING: mining team NOMAINT@HOME-1 can't salvage: it has no maintenance_drone") {
		t.Errorf("no warning about NOMAINT:\n%s", out.String())
	}
	if strings.Contains(out.String(), "BUSY") {
		t.Errorf("mentions BUSY, which has a task:\n%s", out.String())
	}

	// GOOD's maintenance drone wakes its tasks.
	if c, err := store.TeamOf(ctx, db, "M"); err != nil || c != "GOOD" {
		t.Errorf("TeamOf(M) = %q, %v", c, err)
	}
	if c, err := store.TeamOf(ctx, db, "GOOD-D1"); err != nil || c != "GOOD" {
		t.Errorf("TeamOf(GOOD-D1) = %q, %v", c, err)
	}
	if c, err := store.TeamOf(ctx, db, "NOBODY"); err != nil || c != "" {
		t.Errorf("TeamOf(NOBODY) = %q, %v", c, err)
	}
}

func TestSalvageAssignNextAndBlock(t *testing.T) {
	db := salvageStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO tasks (task, location, priority, status, assignee, created_at, updated_at) VALUES
		('salvage', 'HOME-3', 4, 'assigned', 'GOOD', 0, 0)`)
	exec(t, db, `INSERT INTO tasks (task, location, priority, created_at, updated_at) VALUES
		('salvage', 'FAR-2', 4, 0, 0), ('salvage', 'HOME-4', 4, 0, 0)`)
	var wake []int64
	if err := inTx(ctx, db, func(tx *sql.Tx) (err error) {
		wake, err = finishAndNext(ctx, tx, store.Task{ID: 2, Task: salvageTask, Location: "HOME-3"}, "GOOD", "ami_mining_controller", "HOME")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(wake, []int64{4}) {
		t.Errorf("woke %v, want [4] (HOME-4, in the same system)", wake)
	}

	// The move a salvage task files has its priority.
	if err := inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := blockOnMove(store.Task{ID: 4, Priority: salvagePriority}, "HOME-1-L4", "FAR-1-L4", []string{"GOOD"})(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var pri int
	if err := db.QueryRowContext(ctx, `SELECT priority FROM tasks WHERE task = 'move_devices'`).Scan(&pri); err != nil || pri != salvagePriority {
		t.Errorf("move priority %d, %v; want %d", pri, err, salvagePriority)
	}
}

func TestDecideSalvage(t *testing.T) {
	at := func(code, loc string) member { return member{Code: code, Location: loc, Status: "idle"} }
	with := func(m member, f func(*member)) member { f(&m); return m }
	running := func(d string) func(*member) {
		return func(m *member) { m.Directive, m.DirectiveName, m.DirectiveStatus = d, d, "active" }
	}
	completed := func(m *member) { m.DirectiveName, m.DirectiveStatus = salvageDirective, "completed" }
	patrolling := running(patrolDirective)
	state := func(phase string, c, m member, miners ...member) salvageState {
		return salvageState{Target: "FAR-2", TargetEntry: "FAR-1-L4", HomeEntry: "HOME-1-L4", Phase: phase,
			Controller: c, Maint: m, Miners: miners}
	}
	for _, tc := range []struct {
		name   string
		s      salvageState
		step   salvageStep
		device string
		patrol bool
		who    []string
		errHas string
	}{
		{name: "controller travelling", s: state("", at("C", ""), at("M", "HOME-1")), step: salvageWait},
		{name: "drone travelling", s: state("", at("C", "FAR-3"), at("M", "FAR-3"), at("D1", "")), step: salvageWait},
		{name: "split", s: state("", at("C", "FAR-3"), at("M", "FAR-3"), at("D1", "HOME-1")), errHas: "split"},
		{name: "take over", s: state("", with(at("C", "HOME-3"), running("gather_evenly")), at("M", "HOME-1")), step: salvageClear, device: "C"},
		{name: "take over gather_salvage", s: state("", with(at("C", "FAR-3"), running(salvageDirective)), at("M", "FAR-3")), step: salvageClear, device: "C"},
		{name: "maint other directive", s: state("", at("C", "FAR-3"), with(at("M", "FAR-3"), running("repair"))), errHas: "repair"},

		{name: "in target, start", s: state("", at("C", "FAR-3"), at("M", "FAR-1-L4"), at("D1", "FAR-4")), step: salvageStart, patrol: true},
		{name: "in target, start, patrolling", s: state("", at("C", "FAR-3"), with(at("M", ""), patrolling), at("D1", "FAR-4")), step: salvageWait},
		{name: "in target, start, maint patrolling", s: state("", at("C", "FAR-3"), with(at("M", "FAR-5"), patrolling), at("D1", "FAR-4")), step: salvageStart},
		{name: "in target, next site", s: state("", with(at("C", "FAR-3"), completed), with(at("M", "FAR-5"), patrolling), at("D1", "FAR-4")), step: salvageStart},
		{name: "in target, salvaging", s: state(phaseSalvaging, with(at("C", "FAR-3"), running(salvageDirective)), at("M", "FAR-5"), at("D1", "")), step: salvageWait},
		{name: "in target, done", s: state(phaseSalvaging, with(at("C", "FAR-3"), completed), with(at("M", "FAR-5"), patrolling), at("D1", "FAR-2")), step: salvageDone},
		{name: "in target, cleared", s: state(phaseSalvaging, at("C", "FAR-3"), at("M", "FAR-5"), at("D1", "FAR-2")), errHas: "cleared"},
		{name: "in target, attached", s: state("", with(at("C", "FAR-1-L4"), func(m *member) { m.AttachedTo = "T" }), at("M", "FAR-1-L4")), errHas: "attached"},

		{name: "elsewhere, stop patrol first", s: state("", at("C", "HOME-1-L4"), with(at("M", ""), patrolling), at("D1", "HOME-1-L4")), step: salvageClear, device: "M"},
		{name: "elsewhere, gather", s: state("", at("C", "HOME-3"), at("M", "HOME-1-L4"), at("D1", "HOME-4")), step: salvageGather, who: []string{"C", "D1"}},
		{name: "elsewhere, at entry", s: state("", at("C", "HOME-1-L4"), at("M", "HOME-1-L4"), at("D1", "HOME-1-L4")), step: salvageBlock},
		{name: "elsewhere, attached away", s: state("", with(at("C", "HOME-3"), func(m *member) { m.AttachedTo = "T" }), at("M", "HOME-3")), errHas: "attached"},
		{name: "elsewhere, salvaging", s: state(phaseSalvaging, with(at("C", "HOME-3"), running(salvageDirective)), at("M", "HOME-3")), errHas: "outside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, err := decideSalvage(tc.s)
			if tc.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errHas) {
					t.Fatalf("err = %v, want one containing %q (action %+v)", err, tc.errHas, a)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if a.Step != tc.step || a.Device != tc.device || a.Patrol != tc.patrol || !slices.Equal(a.Travellers, tc.who) {
				t.Errorf("got %+v, want step %d device %q patrol %v %v", a, tc.step, tc.device, tc.patrol, tc.who)
			}
			if a.Step == salvageGather && a.Dest != "HOME-1-L4" {
				t.Errorf("gathering at %q", a.Dest)
			}
		})
	}
}
