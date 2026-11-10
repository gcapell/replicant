package main

// Task handlers (design/tasks.md). Each task type has one, named after it in
// camelCase (survey_system -> surveySystem), in that task's own file. A
// handler is a reconciler: it looks at the world, takes the next step for its
// task, and returns, so it must be safe to call again at any point.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"math"
	"time"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/store"
)

// taskHandler takes t's next step. Like an event handler, it may read the
// store and call the API (including commands), but mustn't write to the
// store: it returns any change for the store, which runTasks makes in a
// transaction of its own.
type taskHandler func(ctx context.Context, db store.Reader, t store.Task) (taskChange, error)

// taskChange is a task handler's change to the store. It returns the ids of
// tasks it unblocked or assigned, whose handlers run next.
type taskChange func(ctx context.Context, tx *sql.Tx) (wake []int64, err error)

// taskHandlers maps each task type to its handler.
var taskHandlers = map[string]taskHandler{
	surveyTask:      surveySystem,
	moveDevicesTask: moveDevices,
}

// taskTypesFor lists the task types each device type can be assigned.
// Transports are the types with surge and attach in their blueprints.
var taskTypesFor = map[string][]string{
	"ami_survey_controller": {surveyTask},
	"cargo_vessel":          {moveDevicesTask},
	"mobile_fleet":          {moveDevicesTask},
	"surge_carrier":         {moveDevicesTask},
	"surge_plate":           {moveDevicesTask},
	"surge_platform":        {moveDevicesTask},
}

// taskFits reports whether a device of deviceType can do task t. A task type
// with no entry fits any device taskTypesFor allows.
//
// TODO: survey_system should check the team still has MinSurveyDrones drones.
var taskFits = map[string]func(ctx context.Context, q store.Queryer, t store.Task, deviceType string) (bool, error){
	moveDevicesTask: func(ctx context.Context, q store.Queryer, t store.Task, deviceType string) (bool, error) {
		m, err := parseMove(t)
		if err != nil {
			return false, err
		}
		capacity, err := store.AttachCapacity(ctx, q, deviceType)
		if err != nil {
			return false, err
		}
		return moveFits(m, capacity), nil
	},
}

// sweepTasks is what startup does, and the sweep repeats: file survey tasks
// and give them free teams (fileSurveys), give pending moves free transports
// (assignMoves), then run every assigned task (runAssigned).
func sweepTasks(ctx context.Context, db *sql.DB, w io.Writer) error {
	if err := fileSurveys(ctx, db, w); err != nil {
		return err
	}
	if err := assignMoves(ctx, db, w); err != nil {
		return err
	}
	return runAssigned(ctx, db)
}

// runAssigned calls the handler for every assigned task, highest priority
// first (runTasks). A task type with no handler is an error, before anything
// runs.
func runAssigned(ctx context.Context, db *sql.DB) error {
	ts, err := store.AssignedTasks(ctx, db)
	if err != nil {
		return err
	}
	ids := make([]int64, len(ts))
	for i, t := range ts {
		if taskHandlers[t.Task] == nil {
			return fmt.Errorf("task %d: no handler for task type %q", t.ID, t.Task)
		}
		ids[i] = t.ID
	}
	log.Printf("running %d assigned tasks", len(ts))
	return runTasks(ctx, db, ids)
}

// maxRuns bounds one runTasks call, in case tasks keep waking each other.
const maxRuns = 1000

// runTasks calls the handler of each task in ids, in order, and applies what
// it returns. Tasks that a change unblocks or assigns join the end of the
// queue, so finishing one task starts the next straight away. A task already
// queued isn't queued twice, and a task that's no longer assigned (finished,
// blocked, ...) is skipped. A failing handler stops it.
//
// Everything runs on the caller's goroutine (the event loop), so a task's
// handler never runs twice at once.
func runTasks(ctx context.Context, db *sql.DB, ids []int64) error {
	var queue []int64
	queued := map[int64]bool{}
	add := func(ids ...int64) {
		for _, id := range ids {
			if !queued[id] {
				queue, queued[id] = append(queue, id), true
			}
		}
	}
	add(ids...)
	for runs := 0; len(queue) > 0; runs++ {
		if runs == maxRuns {
			return fmt.Errorf("ran %d task handlers in a row; tasks %v still queued", maxRuns, queue)
		}
		id := queue[0]
		queue = queue[1:]
		delete(queued, id)
		t, ok, err := store.TaskByID(ctx, db, id)
		if err != nil {
			return err
		}
		if !ok || t.Status != "assigned" {
			continue
		}
		wake, err := runTask(ctx, db, t)
		if err != nil {
			return fmt.Errorf("task %d (%s at %s): %w", t.ID, t.Task, t.Location, err)
		}
		add(wake...)
	}
	return nil
}

func runTask(ctx context.Context, db *sql.DB, t store.Task) ([]int64, error) {
	h := taskHandlers[t.Task]
	if h == nil {
		return nil, fmt.Errorf("no handler for task type %q", t.Task)
	}
	c, err := h(ctx, db, t)
	if err != nil || c == nil {
		return nil, err
	}
	var wake []int64
	err = inTx(ctx, db, func(tx *sql.Tx) (err error) {
		wake, err = c(ctx, tx)
		return err
	})
	return wake, err
}

// member is what a task handler knows about one device, fetched from the
// API just before acting.
type member struct {
	Code, Type  string
	Location    string // "" while travelling, attached in transit, or stowed
	Status      string
	AttachedTo  string
	Destination string // while travelling
	Directive   string // the AMI directive it's running ("active"), "" if none or paused
}

// fetchMember reads device code from the API. Its type must be wantType
// (unless that's "") and,
// if controller isn't "", its controller must be controller.
func fetchMember(ctx context.Context, code, wantType, controller string) (member, error) {
	d, err := fetchDevice(ctx, code)
	if err != nil {
		return member{}, err
	}
	if wantType != "" && d.DeviceType != wantType {
		return member{}, fmt.Errorf("device %s is a %s, not a %s", code, d.DeviceType, wantType)
	}
	if controller != "" && d.ControllerDeviceCode != controller {
		return member{}, fmt.Errorf("device %s: controller is %q, not %s", code, d.ControllerDeviceCode, controller)
	}
	m := member{
		Code: code, Type: d.DeviceType, Location: d.Location, Status: d.Status,
		AttachedTo: d.AttachedToDeviceCode, Destination: d.Travel.FinalDestination,
	}
	switch d.AmiDirectiveStatus {
	case "active":
		m.Directive = d.AmiDirective.Name
	case "", "completed", "paused":
		// Not running. A completed directive was seen "paused" after
		// detaching its controller by hand.
	default:
		return member{}, fmt.Errorf("device %s: directive %s has status %q", code, d.AmiDirective.Name, d.AmiDirectiveStatus)
	}
	return m, nil
}

// assignNext is "what should device do next?": of the pending, unassigned
// tasks its type can do (taskTypesFor) and that fit it (taskFits), it takes
// the highest priority one, nearest star first. device is at star. It
// returns the task (now assigned), or nil if there's nothing for it.
func assignNext(ctx context.Context, tx *sql.Tx, device, deviceType, star string, now time.Time) (*store.Task, error) {
	pending, err := store.PendingTasks(ctx, tx, taskTypesFor[deviceType]...)
	if err != nil {
		return nil, err
	}
	here, err := store.StarPosition(ctx, tx, star)
	if err != nil {
		return nil, err
	}
	var best *store.Task
	bestDist := math.Inf(1)
	for i, t := range pending {
		if best != nil && t.Priority > best.Priority {
			break
		}
		if fits := taskFits[t.Task]; fits != nil {
			ok, err := fits(ctx, tx, t, deviceType)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		p, err := store.StarPosition(ctx, tx, auto.StarOf(t.Location))
		if err != nil {
			return nil, err
		}
		if d := here.Distance(p); d < bestDist {
			best, bestDist = &pending[i], d
		}
	}
	if best == nil {
		log.Printf("%s (%s): no pending task it can do", device, deviceType)
		return nil, nil
	}
	if err := store.AssignTask(ctx, tx, best.ID, device, now); err != nil {
		return nil, err
	}
	log.Printf("%s (%s): assigned task %d %s %s (%.1f ly)", device, deviceType, best.ID, best.Task, best.Location, bestDist)
	return best, nil
}

// setPhase is a change recording that task t is now in phase, or nil if it
// already is.
func setPhase(t store.Task, phase string) taskChange {
	if t.Phase == phase {
		return nil
	}
	return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
		log.Printf("task %d %s %s: phase %s", t.ID, t.Task, t.Location, phase)
		return nil, store.SetPhase(ctx, tx, t.ID, phase, time.Now())
	}
}

// finishAndNext archives task t as done, then gives its assignee (a
// deviceType at star) its next task. It returns the tasks to run next: those
// t's finishing unblocked, and the assignee's new task.
func finishAndNext(ctx context.Context, tx *sql.Tx, t store.Task, device, deviceType, star string) ([]int64, error) {
	now := time.Now()
	wake, err := store.FinishTask(ctx, tx, t.ID, now)
	if err != nil {
		return nil, err
	}
	log.Printf("task %d %s %s done; unblocked %v", t.ID, t.Task, t.Location, wake)
	next, err := assignNext(ctx, tx, device, deviceType, star, now)
	if err != nil {
		return nil, err
	}
	if next != nil {
		wake = append(wake, next.ID)
	}
	return wake, nil
}
