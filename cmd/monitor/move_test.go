package main

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func moveTask(id int64, loc string, n int) store.Task {
	devs := make([]string, n)
	for i := range devs {
		devs[i] = fmt.Sprintf(`"D%d_%d"`, id, i)
	}
	return store.Task{ID: id, Task: moveDevicesTask, Location: loc, Status: "pending",
		Details: fmt.Sprintf(`{"dst":"FAR-1-L4","devices":[%s]}`, strings.Join(devs, ","))}
}

func TestPlanMoves(t *testing.T) {
	pos := map[string]model.Position{"A": {X: 0}, "B": {X: 4}, "C": {X: 10}}
	tasks := []store.Task{moveTask(1, "A-1-L4", 7), moveTask(2, "C-1-L4", 1), moveTask(3, "B-1-L4", 20)}
	transports := []store.Transport{
		{Code: "BIG", Star: "C", Capacity: 9},   // only it can take task 1
		{Code: "PLATE", Star: "A", Capacity: 1}, // nearer task 1, but too small
	}
	p, err := planMoves(tasks, transports, pos)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range p.Assign {
		got = append(got, fmt.Sprintf("%d<-%s %.0f", a.Task.ID, a.Transport.Code, a.Distance))
	}
	// BIG takes task 1 (10 ly), PLATE task 2 (10 ly); task 3 is too big for anyone.
	if want := []string{"1<-BIG 10", "2<-PLATE 10"}; !slices.Equal(got, want) {
		t.Errorf("Assign = %v, want %v", got, want)
	}
	if len(p.Waiting) != 1 || p.Waiting[0].ID != 3 {
		t.Errorf("Waiting = %+v, want task 3", p.Waiting)
	}

	bad := store.Task{ID: 9, Task: moveDevicesTask, Location: "A-1-L4", Details: `{"dst":"X"}`}
	if _, err := planMoves([]store.Task{bad}, transports, pos); err == nil {
		t.Error("no error for a move with no devices")
	}
}

// moveStore has stars HOME, NEAR, FAR, transports in various states, and a
// pending move from NEAR.
func moveStore(t *testing.T) *sql.DB {
	t.Helper()
	db := taskStore(t)
	ctx := t.Context()
	if err := store.ReplaceBlueprints(ctx, db, []model.Blueprint{
		{DeviceType: "surge_carrier", AttachCapacity: 9},
		{DeviceType: "surge_plate", AttachCapacity: 1},
	}); err != nil {
		t.Fatal(err)
	}
	carrier := func(code, loc string) model.Device {
		return model.Device{DeviceCode: code, DeviceType: "surge_carrier", Status: "idle", Location: loc, Features: []string{"surge", "cruise", "attach"}}
	}
	devices := []model.Device{
		carrier("FREE", "FAR-2"),
		carrier("NEARBY", "HOME-2"),
		carrier("LOADED", "NEAR-1"), // carrying CARGO
		{DeviceCode: "CARGO", DeviceType: "survey_drone", Status: "idle", Location: "NEAR-1", AttachedToDeviceCode: "LOADED"},
		carrier("MOVING", ""),
		carrier("BUSY", "NEAR-1"), // assigned below
		carrier("CLAIMED", "NEAR-1"),
		carrier("PASSENGER", "NEAR-1-L4"), // one of the devices to move
		{DeviceCode: "PLATE", DeviceType: "surge_plate", Status: "idle", Location: "NEAR-1", Features: []string{"surge", "cruise", "attach", "stow", "taxi"}},
		{DeviceCode: "BARGE", DeviceType: "fusion_barge", Status: "idle", Location: "NEAR-1", Features: []string{"cruise", "attach", "transport"}},
	}
	for _, d := range devices {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	exec(t, db, `INSERT INTO tasks (task, location, status, assignee, details, created_at, updated_at) VALUES
		('move_devices', 'NEAR-1-L4', 'pending', NULL, '{"dst":"FAR-1-L4","devices":["C1","D1","PASSENGER"]}', 0, 0),
		('survey_system', 'FAR', 'assigned', 'BUSY', NULL, 0, 0),
		('survey_system', 'HOME', 'pending', NULL, NULL, 0, 0)`)
	exec(t, db, `INSERT INTO claimed_devices (device, task) VALUES ('CLAIMED', 3)`)
	return db
}

func TestFreeTransports(t *testing.T) {
	db := moveStore(t)
	ts, err := store.FreeTransports(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range ts {
		got = append(got, fmt.Sprintf("%s %s %d", x.Code, x.Star, x.Capacity))
	}
	if want := []string{"FREE FAR 9", "NEARBY HOME 9", "PLATE NEAR 1"}; !slices.Equal(got, want) {
		t.Errorf("free transports = %q, want %q", got, want)
	}
}

func TestAssignMoves(t *testing.T) {
	db := moveStore(t)
	ctx := t.Context()
	var out strings.Builder
	if err := assignMoves(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	// PLATE is nearest but too small; NEARBY (1 ly) beats FREE (8 ly).
	if got := taskRows(t, db, "tasks"); !strings.HasPrefix(got[0], "1 move_devices NEAR-1-L4 assigned NEARBY") {
		t.Errorf("tasks = %q\n%s", got, out.String())
	}
	// Again: nothing pending.
	out.Reset()
	if err := assignMoves(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "move tasks: 0 to assign, 0 left pending") {
		t.Errorf("second run printed %q", out.String())
	}
}

func TestAssignNextFits(t *testing.T) {
	db := moveStore(t)
	ctx := t.Context()
	// A plate can't carry 3 devices; a carrier can.
	for _, c := range []struct {
		device, typ string
		want        bool
	}{{"PLATE", "surge_plate", false}, {"FREE", "surge_carrier", true}} {
		err := inTx(ctx, db, func(tx *sql.Tx) error {
			got, err := assignNext(ctx, tx, c.device, c.typ, "HOME", time.Now())
			if (got != nil) != c.want {
				t.Errorf("%s: assigned %+v, want any: %v", c.device, got, c.want)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecideMove(t *testing.T) {
	const pickup, dst = "NEAR-1-L4", "FAR-1-L4"
	devs := []string{"C1", "D1"}
	tr := func(loc string, attached ...string) carrier {
		return carrier{member: member{Code: "T", Location: loc, Status: "idle"}, Attached: attached, Free: 9 - len(attached)}
	}
	p := func(code, loc, attachedTo string) member {
		return member{Code: code, Location: loc, AttachedTo: attachedTo, Status: "idle"}
	}
	waiting := []member{p("C1", pickup, ""), p("D1", pickup, "")}
	for _, c := range []struct {
		name       string
		t          carrier
		passengers []member
		step       moveStep
		attach     []string
		errHas     string
	}{
		{name: "travelling", t: tr(""), step: moveWait},
		{name: "elsewhere, empty", t: tr("HOME-2"), step: moveToPickup},
		{name: "at pickup, empty", t: tr(pickup), passengers: waiting, step: moveLoad, attach: devs},
		{name: "at pickup, half loaded", t: tr(pickup, "C1"), passengers: []member{p("C1", pickup, "T"), p("D1", pickup, "")}, step: moveLoad, attach: []string{"D1"}},
		{name: "at pickup, loaded", t: tr(pickup, "C1", "D1"), passengers: []member{p("C1", pickup, "T"), p("D1", pickup, "T")}, step: moveToDst},
		{name: "elsewhere, loaded", t: tr("HOME-2", "D1", "C1"), step: moveToDst},
		{name: "at dst, loaded", t: tr(dst, "C1", "D1"), step: moveUnload},
		{name: "at dst, unloaded", t: tr(dst), passengers: []member{p("C1", dst, ""), p("D1", dst, "")}, step: moveFinish},

		{name: "passenger missing", t: tr(pickup), passengers: []member{p("C1", pickup, ""), p("D1", "", "")}, errHas: "not waiting"},
		{name: "passenger elsewhere", t: tr(pickup), passengers: []member{p("C1", pickup, ""), p("D1", "NEAR-3", "")}, errHas: "D1"},
		{name: "passenger on another carrier", t: tr(pickup), passengers: []member{p("C1", pickup, "OTHER"), p("D1", pickup, "")}, errHas: "OTHER"},
		{name: "no room", t: carrier{member: member{Code: "T", Location: pickup}, Free: 1}, passengers: waiting, errHas: "room"},
		{name: "stranger aboard", t: tr("HOME-2", "X"), errHas: "X"},
		{name: "partly loaded away", t: tr("HOME-2", "C1"), errHas: "some"},
		{name: "partly loaded at dst", t: tr(dst, "C1"), errHas: "only"},
		{name: "at dst, passengers not there", t: tr(dst), passengers: []member{p("C1", dst, ""), p("D1", pickup, "")}, errHas: "D1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, err := decideMove(moveState{Pickup: pickup, Dst: dst, Devices: devs, Transport: c.t, Passengers: c.passengers})
			if c.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), c.errHas) {
					t.Fatalf("err = %v, want one containing %q (action %+v)", err, c.errHas, a)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if a.Step != c.step || !slices.Equal(a.Attach, c.attach) {
				t.Errorf("got %+v, want step %d attach %v", a, c.step, c.attach)
			}
		})
	}
}

func TestFinishMove(t *testing.T) {
	db := moveStore(t)
	ctx := t.Context()
	// The move is assigned to FREE, and blocks a survey of FAR assigned to C9.
	exec(t, db, `UPDATE tasks SET status = 'assigned', assignee = 'FREE' WHERE id = 1`)
	exec(t, db, `INSERT INTO tasks (id, task, location, status, assignee, created_at, updated_at)
		VALUES (10, 'survey_system', 'NEAR', 'blocked', 'C9', 0, 0)`)
	exec(t, db, `INSERT INTO blockers (task, dep) VALUES (10, 1)`)
	exec(t, db, `INSERT INTO tasks (task, location, status, details, created_at, updated_at)
		VALUES ('move_devices', 'HOME-1-L4', 'pending', '{"dst":"FAR-1-L4","devices":["X"]}', 0, 0)`)

	task := store.Task{ID: 1, Task: moveDevicesTask, Location: "NEAR-1-L4", Status: "assigned", Assignee: "FREE"}
	tr := carrier{member: member{Code: "FREE", Type: "surge_carrier", Location: "FAR-1-L4"}}
	var wake []int64
	if err := inTx(ctx, db, func(tx *sql.Tx) (err error) { wake, err = finishMove(task, tr)(ctx, tx); return err }); err != nil {
		t.Fatal(err)
	}
	// The unblocked survey, then the transport's next move.
	if !slices.Equal(wake, []int64{10, 11}) {
		t.Errorf("wake = %v, want [10 11]", wake)
	}
	got := taskRows(t, db, "tasks")
	want := []string{
		"2 survey_system FAR assigned BUSY",
		"3 survey_system HOME pending",
		"10 survey_system NEAR assigned C9",
		`11 move_devices HOME-1-L4 assigned FREE {"dst":"FAR-1-L4","devices":["X"]}`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
	if got := taskRows(t, db, "archived_tasks"); len(got) != 1 || !strings.HasPrefix(got[0], "1 move_devices NEAR-1-L4 done FREE") {
		t.Errorf("archived = %q", got)
	}
}
