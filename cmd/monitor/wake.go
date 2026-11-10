package main

// Which events wake which tasks. A task's handler is a reconciler, so a wake
// it didn't need costs only API calls; a wake it missed is caught by the
// sweep.

import (
	"context"
	"slices"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/store"
)

// wakeRule lists, for one task type, the events that wake its tasks. Each
// list is of event names, e.g. travel.arrived.
type wakeRule struct {
	Assignee []string // events whose device is the task's assignee
	Team     []string // events whose device is in the assignee's team (store.TeamOf: an AMI controller's drones, a mining controller's maintenance_drone)
	Star     []string // events at the task's star
	Location []string // events at exactly the task's location
}

// taskWakes is the wakeRule for each task type. Only assigned tasks wake: a
// blocked one waits to be unblocked, which runs it.
var taskWakes = map[string]wakeRule{
	// The transport's own arrival. Its passengers get no events of their
	// own while carried.
	moveDevicesTask: {Assignee: []string{"travel.arrived", "travel.cancelled"}},
	// The controller's directive ending, and the team's own trips to an
	// entry point (gathering, regrouping). While a transport carries the
	// team the survey is blocked, and finishing the move runs it.
	surveyTask: {
		Assignee: []string{"directive.completed", "directive.cleared", "travel.arrived"},
		Team:     []string{"travel.arrived"},
	},
	// As for surveys, plus the maintenance_drone's patrol being cleared
	// before the team leaves a system.
	salvageTask: {
		Assignee: []string{"directive.completed", "directive.cleared", "travel.arrived"},
		Team:     []string{"travel.arrived", "directive.cleared"},
	},
	// The observatory finishing a compact, unfurl or prospect. Its trips
	// between systems are move_devices tasks, whose finishing runs it.
	gazeTask: {Assignee: []string{"device.compacted", "device.unfurled", "prospect.completed"}},
}

// wakes reports whether event e wakes task t. controller is the controller
// of e's device ("" if none).
func (w wakeRule) wakes(t store.Task, e api.Envelope, controller string) bool {
	in := func(names []string) bool { return slices.Contains(names, e.Event) }
	switch {
	case e.DeviceCode != "" && e.DeviceCode == t.Assignee && in(w.Assignee):
		return true
	case controller != "" && controller == t.Assignee && in(w.Team):
		return true
	case e.Star != "" && e.Star == auto.StarOf(t.Location) && in(w.Star):
		return true
	case e.Location != "" && e.Location == t.Location && in(w.Location):
		return true
	}
	return false
}

// tasksToWake lists the assigned tasks that event e wakes, in priority order.
func tasksToWake(ctx context.Context, q store.Reader, e api.Envelope) ([]int64, error) {
	ts, err := store.AssignedTasks(ctx, q)
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	controller := ""
	if e.DeviceCode != "" {
		if controller, err = store.TeamOf(ctx, q, e.DeviceCode); err != nil {
			return nil, err
		}
	}
	var ids []int64
	for _, t := range ts {
		if taskWakes[t.Task].wakes(t, e, controller) {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}
