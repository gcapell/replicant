package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

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
	taskHandlers[surveyTask] = func(_ context.Context, task store.Task) error {
		got = append(got, fmt.Sprintf("%s %d %s %s", task.Location, task.Priority, task.Assignee, task.Details))
		return nil
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
