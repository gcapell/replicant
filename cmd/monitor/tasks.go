package main

// Task handlers (design/tasks.md). Each task type has one, named after it in
// camelCase (survey_system -> surveySystem), in that task's own file. A
// handler is a reconciler: it looks at the world, takes the next step for its
// task, and returns, so it must be safe to call again at any point.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/store"
)

// taskHandler takes t's next step. Like an event handler, it may read the
// store and call the API (including commands), but mustn't write to the
// store: it returns any change for the store, which runAssigned makes in a
// transaction of its own.
type taskHandler func(ctx context.Context, db store.Reader, t store.Task) (change, error)

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

// runAssigned calls the handler for every assigned task, highest priority
// first, once each, and applies what it returns. A task type with no handler
// is an error, as is a failing handler. Tasks that a handler assigns or
// unblocks wait for their next wake-up (TODO: wake them).
func runAssigned(ctx context.Context, db *sql.DB) error {
	ts, err := store.AssignedTasks(ctx, db)
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
		if err := runTask(ctx, db, t); err != nil {
			return fmt.Errorf("task %d (%s at %s): %w", t.ID, t.Task, t.Location, err)
		}
	}
	return nil
}

func runTask(ctx context.Context, db *sql.DB, t store.Task) error {
	c, err := taskHandlers[t.Task](ctx, db, t)
	if err != nil || c == nil {
		return err
	}
	return inTx(ctx, db, func(tx *sql.Tx) error { return c(ctx, tx) })
}

// member is what a task handler knows about one device, fetched from the
// API just before acting.
type member struct {
	Code, Type  string
	Location    string // "" while travelling, attached in transit, or stowed
	Status      string
	AttachedTo  string
	Destination string // while travelling
	Directive   string // the AMI directive it's running, "" if none
}

// fetchMember reads device code from the API. Its type must be wantType and,
// if controller isn't "", its controller must be controller.
func fetchMember(ctx context.Context, code, wantType, controller string) (member, error) {
	d, err := api.Device(ctx, code)
	if err != nil {
		return member{}, err
	}
	if d.DeviceType != wantType {
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
	case "", "completed":
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
