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

func TestWakeRule(t *testing.T) {
	w := wakeRule{Assignee: []string{"a.ev"}, Team: []string{"t.ev"}, Star: []string{"s.ev"}, Location: []string{"l.ev"}}
	task := store.Task{Location: "FAR-1-L4", Assignee: "C"}
	for _, c := range []struct {
		name       string
		e          api.Envelope
		controller string
		want       bool
	}{
		{"assignee", api.Envelope{Event: "a.ev", DeviceCode: "C"}, "", true},
		{"assignee, other event", api.Envelope{Event: "t.ev", DeviceCode: "C"}, "", false},
		{"other device", api.Envelope{Event: "a.ev", DeviceCode: "X"}, "", false},
		{"team", api.Envelope{Event: "t.ev", DeviceCode: "D1"}, "C", true},
		{"other team", api.Envelope{Event: "t.ev", DeviceCode: "D1"}, "Z", false},
		{"star", api.Envelope{Event: "s.ev", Star: "FAR"}, "", true},
		{"other star", api.Envelope{Event: "s.ev", Star: "NEAR"}, "", false},
		{"location", api.Envelope{Event: "l.ev", Location: "FAR-1-L4"}, "", true},
		{"same star, other location", api.Envelope{Event: "l.ev", Location: "FAR-2", Star: "FAR"}, "", false},
		{"no device", api.Envelope{Event: "a.ev"}, "", false},
	} {
		if got := w.wakes(task, c.e, c.controller); got != c.want {
			t.Errorf("%s: wakes = %v, want %v", c.name, got, c.want)
		}
	}
}

// fakeTaskHandlers replaces the task handlers with ones that record which
// tasks ran, and do what each task's step says (see wakeStore).
func fakeTaskHandlers(t *testing.T) *[]string {
	t.Helper()
	var ran []string
	saved := taskHandlers
	t.Cleanup(func() { taskHandlers = saved })
	h := func(_ context.Context, _ store.Reader, task store.Task) (taskChange, error) {
		ran = append(ran, fmt.Sprintf("%d", task.ID))
		if task.Details != "finish" {
			return nil, nil
		}
		// Finish, like a move whose passengers are unloaded.
		return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
			return finishAndNext(ctx, tx, task, task.Assignee, "surge_carrier", "HOME")
		}, nil
	}
	taskHandlers = map[string]taskHandler{surveyTask: h, moveDevicesTask: h}
	return &ran
}

// wakeStore: move 1 (transport T1, finishes when run) blocks survey 2
// (controller C1, whose drone is D1). Move 3 is pending for T1 afterwards.
func wakeStore(t *testing.T) *sql.DB {
	t.Helper()
	db := moveStore(t)
	exec(t, db, `DELETE FROM tasks`)
	exec(t, db, `INSERT INTO tasks (id, task, location, status, assignee, details, created_at, updated_at) VALUES
		(1, 'move_devices', 'NEAR-1-L4', 'assigned', 'T1', 'finish', 0, 0),
		(2, 'survey_system', 'FAR', 'blocked', 'C1', NULL, 0, 0),
		(3, 'move_devices', 'HOME-1-L4', 'pending', NULL, '{"dst":"FAR-1-L4","devices":["X"]}', 0, 0)`)
	exec(t, db, `INSERT INTO blockers (task, dep) VALUES (2, 1)`)
	if err := store.PutDevice(t.Context(), db, model.Device{DeviceCode: "D1", DeviceType: "survey_drone", Status: "idle", Location: "FAR-1", ControllerDeviceCode: "C1"}); err != nil {
		t.Fatal(err)
	}
	return db
}

// Finishing the move runs the survey it unblocked, then the transport's
// next move.
func TestRunTasksChains(t *testing.T) {
	db := wakeStore(t)
	ran := fakeTaskHandlers(t)
	if err := runTasks(t.Context(), db, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	// 1 finishes, unblocking 2 (already queued, so not queued again) and
	// assigning 3 to T1, which joins the queue.
	if want := []string{"1", "2", "3"}; !slices.Equal(*ran, want) {
		t.Errorf("ran %v, want %v", *ran, want)
	}
	got := taskRows(t, db, "tasks")
	want := []string{"2 survey_system FAR assigned C1", `3 move_devices HOME-1-L4 assigned T1 {"dst":"FAR-1-L4","devices":["X"]}`}
	if !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
}

func TestExecuteWakesTasks(t *testing.T) {
	db := wakeStore(t)
	exec(t, db, `UPDATE tasks SET details = NULL WHERE id = 1`) // the move doesn't finish
	exec(t, db, `UPDATE tasks SET status = 'assigned' WHERE id = 2`)
	exec(t, db, `DELETE FROM blockers`)
	ran := fakeTaskHandlers(t)
	// Event names monitor doesn't apply, so they need no payloads.
	saved := taskWakes
	t.Cleanup(func() { taskWakes = saved })
	taskWakes = map[string]wakeRule{
		moveDevicesTask: {Assignee: []string{"test.arrived"}},
		surveyTask:      {Assignee: []string{"test.done"}, Team: []string{"test.arrived"}},
	}
	ctx := t.Context()
	n := 0
	send := func(replaying bool, events ...api.Envelope) error {
		var rs []received
		for _, e := range events {
			n++
			full := ev(fmt.Sprintf("%d-0", 1000+n), e.Event)
			full.DeviceCode = e.DeviceCode
			rs = append(rs, received{full, nil})
		}
		return execute(ctx, db, feed(rs...), &printer{w: &strings.Builder{}}, replaying, &executor{})
	}
	// Unhandled event names, so applying them changes nothing.
	if err := send(false,
		api.Envelope{Event: "test.arrived", DeviceCode: "T1"},    // wakes the move
		api.Envelope{Event: "test.arrived", DeviceCode: "D1"},    // the survey's drone
		api.Envelope{Event: "test.other", DeviceCode: "C1"},      // not one the survey wants
		api.Envelope{Event: "test.arrived", DeviceCode: "OTHER"}, // nobody's
	); err != nil {
		t.Fatal(err)
	}
	if want := []string{"1", "2"}; !slices.Equal(*ran, want) {
		t.Errorf("ran %v, want %v", *ran, want)
	}
	*ran = nil
	if err := send(true, api.Envelope{Event: "test.arrived", DeviceCode: "T1"}); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 0 {
		t.Errorf("replay ran %v", *ran)
	}
	// Without an executor (recordAll), nothing runs.
	e := ev("2000-0", "test.arrived")
	e.DeviceCode = "T1"
	if err := recordAll(ctx, db, feed(received{e, nil}), &printer{w: &strings.Builder{}}, false); err != nil {
		t.Fatal(err)
	}
	if len(*ran) != 0 {
		t.Errorf("recordAll ran %v", *ran)
	}
}

func TestExecuteSweeps(t *testing.T) {
	db := wakeStore(t)
	exec(t, db, `UPDATE tasks SET details = NULL WHERE id = 1`)
	ran := fakeTaskHandlers(t)
	sweep := make(chan time.Time) // unbuffered: the send returns once the loop has taken it
	in := make(chan received)
	done := make(chan error)
	go func() {
		done <- execute(t.Context(), db, in, &printer{w: &strings.Builder{}}, false, &executor{sweep: sweep})
	}()
	sweep <- time.Now()
	// The loop takes this only once the sweep's handlers have run.
	in <- received{ev("1-0", "x"), nil}
	close(in)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// 1 was assigned; the sweep gives pending move 3 a free transport first.
	if want := []string{"1", "3"}; !slices.Equal(*ran, want) {
		t.Errorf("sweep ran %v, want %v", *ran, want)
	}
}
