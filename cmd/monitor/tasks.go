package main

// Task handlers (design/tasks.md). Each task type has one, named after it in
// camelCase (survey_system -> surveySystem). A handler is a reconciler: it
// looks at the world, takes the next step for its task, and returns, so it
// must be safe to call again at any point.

import (
	"context"
	"fmt"
	"log"

	"github.com/gcapell/replicant/store"
)

type taskHandler func(ctx context.Context, t store.Task) error

// taskHandlers maps each task type to its handler.
var taskHandlers = map[string]taskHandler{
	surveyTask: surveySystem,
}

// runAssigned calls the handler for every assigned task, highest priority
// first. A task type with no handler is an error, as is a failing handler.
func runAssigned(ctx context.Context, q store.Reader) error {
	ts, err := store.AssignedTasks(ctx, q)
	if err != nil {
		return err
	}
	for _, t := range ts {
		if taskHandlers[t.Task] == nil {
			return fmt.Errorf("task %d: no handler for task type %q", t.ID, t.Task)
		}
	}
	log.Printf("running %d assigned tasks", len(ts))
	for _, t := range ts {
		if err := taskHandlers[t.Task](ctx, t); err != nil {
			return fmt.Errorf("task %d (%s at %s): %w", t.ID, t.Task, t.Location, err)
		}
	}
	return nil
}
