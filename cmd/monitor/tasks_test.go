package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestRunAssigned(t *testing.T) {
	db := openStore(t)
	exec(t, db, `INSERT INTO tasks (task, location, priority, status, assignee, details, created_at, updated_at) VALUES
		('survey_system', 'LOW', 4, 'assigned', 'C1', NULL, 0, 0),
		('survey_system', 'HIGH', 1, 'assigned', 'C2', '{"x":1}', 0, 0),
		('survey_system', 'WAITING', 0, 'pending', NULL, NULL, 0, 0),
		('survey_system', 'BLOCKED', 0, 'blocked', 'C3', NULL, 0, 0),
		('survey_system', 'BROKEN', 0, 'failed', 'C4', NULL, 0, 0),
		('survey_system', 'HIGH2', 1, 'assigned', 'C5', NULL, 0, 0)`)

	var got []string
	saved := taskHandlers[surveyTask]
	t.Cleanup(func() { taskHandlers[surveyTask] = saved })
	taskHandlers[surveyTask] = func(_ context.Context, _ store.Reader, task store.Task) (change, error) {
		got = append(got, fmt.Sprintf("%s %d %s %s", task.Location, task.Priority, task.Assignee, task.Details))
		return nil, nil
	}
	if err := runAssigned(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	want := []string{`HIGH 1 C2 {"x":1}`, "HIGH2 1 C5 ", "LOW 4 C1 "}
	if !slices.Equal(got, want) {
		t.Errorf("called with %q, want %q", got, want)
	}

	// A task type with no handler stops it before any handler runs.
	exec(t, db, `INSERT INTO tasks (task, location, status, assignee, created_at, updated_at)
		VALUES ('juggle', 'X', 'assigned', 'C6', 0, 0)`)
	got = nil
	err := runAssigned(t.Context(), db)
	if err == nil || !strings.Contains(err.Error(), "juggle") {
		t.Errorf("err = %v, want one about juggle", err)
	}
	if len(got) != 0 {
		t.Errorf("handlers ran: %q", got)
	}
}

// taskStore has stars HOME (x=0), NEAR (x=1) and FAR (x=9).
func taskStore(t *testing.T) *sql.DB {
	t.Helper()
	db := openStore(t)
	exec(t, db, `INSERT INTO stars (designation, x, y, z) VALUES ('HOME', 0, 0, 0), ('NEAR', 1, 0, 0), ('FAR', 9, 0, 0)`)
	return db
}

func taskRows(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT id, task, coalesce(location, ''), status, coalesce(assignee, ''), coalesce(details, '') FROM `+table+` ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id int64
		var task, loc, status, assignee, details string
		if err := rows.Scan(&id, &task, &loc, &status, &assignee, &details); err != nil {
			t.Fatal(err)
		}
		got = append(got, strings.TrimSpace(fmt.Sprintf("%d %s %s %s %s %s", id, task, loc, status, assignee, details)))
	}
	return got
}

func TestAssignNext(t *testing.T) {
	db := taskStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO tasks (task, location, priority, status, assignee, created_at, updated_at) VALUES
		('survey_system', 'FAR', 3, 'pending', NULL, 0, 0),
		('survey_system', 'NEAR', 3, 'pending', NULL, 0, 0),
		('survey_system', 'HOME', 4, 'pending', NULL, 0, 0),
		('move_devices', 'HOME-1-L4', 0, 'pending', NULL, 0, 0)`)
	// Priority 3 beats 4; NEAR is nearer than FAR. move_devices isn't for a controller.
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		got, err := assignNext(ctx, tx, "C1", "ami_survey_controller", "HOME", time.Now())
		if err == nil && (got == nil || got.Location != "NEAR") {
			t.Errorf("assigned %+v, want NEAR", got)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Nothing for a type with no task types.
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		got, err := assignNext(ctx, tx, "X", "mining_drone", "HOME", time.Now())
		if got != nil {
			t.Errorf("assigned %+v to a mining_drone", got)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1 survey_system FAR pending", "2 survey_system NEAR assigned C1", "3 survey_system HOME pending", "4 move_devices HOME-1-L4 pending"}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
}

func TestBlockOnMoveThenFinish(t *testing.T) {
	db := taskStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO tasks (task, location, status, assignee, created_at, updated_at)
		VALUES ('survey_system', 'FAR', 'assigned', 'C1', 0, 0)`)
	if err := inTx(ctx, db, func(tx *sql.Tx) error {
		return blockOnMove(1, "HOME-1-L4", "FAR-2-L4", []string{"C1", "D1"})(ctx, tx)
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1 survey_system FAR blocked C1",
		`2 move_devices HOME-1-L4 pending  {"dst":"FAR-2-L4","devices":["C1","D1"]}`,
	}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}

	// Finishing the move unblocks the survey, which keeps its assignee.
	exec(t, db, `UPDATE tasks SET status = 'assigned', assignee = 'T1' WHERE id = 2`)
	var unblocked []int64
	if err := inTx(ctx, db, func(tx *sql.Tx) (err error) {
		unblocked, err = store.FinishTask(ctx, tx, 2, time.Now())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(unblocked, []int64{1}) {
		t.Errorf("unblocked %v, want [1]", unblocked)
	}
	if got, want := taskRows(t, db, "tasks"), []string{"1 survey_system FAR assigned C1"}; !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
	if got := taskRows(t, db, "archived_tasks"); len(got) != 1 || !strings.HasPrefix(got[0], "2 move_devices HOME-1-L4 done T1") {
		t.Errorf("archived = %q", got)
	}
	if n, err := store.Count(ctx, db, "blockers"); err != nil || n != 0 {
		t.Errorf("blockers: %d, %v", n, err)
	}

	// Only an assigned task can be finished.
	exec(t, db, `UPDATE tasks SET status = 'pending', assignee = NULL WHERE id = 1`)
	if err := inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := store.FinishTask(ctx, tx, 1, time.Now())
		return err
	}); err == nil {
		t.Error("finished a pending task")
	}
}

func TestFetchMemberDirectiveStatus(t *testing.T) {
	status := ""
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		d := &model.Device{DeviceCode: code, DeviceType: "ami_survey_controller", AmiDirectiveStatus: status}
		d.AmiDirective.Name = "survey_system"
		return d, nil
	}
	defer func() { fetchDevice = api.Device }()
	for _, c := range []struct{ status, directive string }{
		{"active", "survey_system"}, {"completed", ""}, {"paused", ""}, {"", ""},
	} {
		status = c.status
		m, err := fetchMember(t.Context(), "C", "", "")
		if err != nil || m.Directive != c.directive {
			t.Errorf("status %q: directive %q, err %v; want %q", c.status, m.Directive, err, c.directive)
		}
	}
	status = "exploded"
	if _, err := fetchMember(t.Context(), "C", "", ""); err == nil {
		t.Error("unknown status accepted")
	}
}

func TestSetPhase(t *testing.T) {
	db := taskStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO tasks (task, location, status, assignee, created_at, updated_at) VALUES
		('move_devices', 'NEAR-1-L4', 'assigned', 'T1', 0, 0),
		('survey_system', 'FAR', 'blocked', 'C1', 0, 0)`)
	task := store.Task{ID: 1, Task: moveDevicesTask, Location: "NEAR-1-L4"}
	if err := inTx(ctx, db, func(tx *sql.Tx) error { return setPhase(task, phaseToPickup)(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	ts, err := store.AssignedTasks(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].Phase != phaseToPickup {
		t.Errorf("assigned tasks = %+v, want task 1 in phase %s", ts, phaseToPickup)
	}
	var updated int64
	db.QueryRow(`SELECT updated_at FROM tasks WHERE id = 1`).Scan(&updated)
	if updated == 0 {
		t.Error("updated_at not set")
	}
	// Already in that phase: nothing to do.
	if c := setPhase(ts[0], phaseToPickup); c != nil {
		t.Error("setPhase to the current phase returned a change")
	}
	// Only an assigned task has a phase set.
	blocked := store.Task{ID: 2, Task: surveyTask, Location: "FAR"}
	if err := inTx(ctx, db, func(tx *sql.Tx) error { return setPhase(blocked, phaseSurveying)(ctx, tx) }); err == nil {
		t.Error("set the phase of a blocked task")
	}
}
