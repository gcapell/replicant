package main

// mine tasks (design/tasks.md): a continual task holding an
// ami_mining_controller that's mining a belt (gather_evenly), so nothing
// else takes it. With -add_miners, startup files one for each controller
// already doing so. The handler does nothing.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

const (
	mineTask        = "mine"
	gatherDirective = "gather_evenly"
)

// mine is the mine task's handler. Mining runs itself (gather_evenly), so it
// does nothing.
func mine(context.Context, store.Reader, store.Task) (taskChange, error) { return nil, nil }

// listDevices is auto.Devices, replaceable in tests.
var listDevices = auto.Devices

// miner is a controller mining a belt.
type miner struct{ Code, Location string }

// chooseMiners picks the ami_mining_controllers whose AMI directive is
// gather_evenly. One of those that isn't at a belt (-BELT-) is an error, as
// is a device of another type.
func chooseMiners(devs []model.Device) ([]miner, error) {
	var ms []miner
	for _, d := range devs {
		if d.DeviceType != "ami_mining_controller" {
			return nil, fmt.Errorf("listing ami_mining_controllers gave %s, a %s", d.DeviceCode, d.DeviceType)
		}
		if d.AmiDirective.Name != gatherDirective {
			continue
		}
		if !strings.Contains(d.Location, "-BELT-") {
			return nil, fmt.Errorf("%s is running %s at %q, not at a belt", d.DeviceCode, gatherDirective, d.Location)
		}
		ms = append(ms, miner{d.DeviceCode, d.Location})
	}
	return ms, nil
}

// addMiners files a mine task at its belt, assigned to it, for each
// ami_mining_controller running gather_evenly (from the API) that has no
// task. One that already has a mine task is left alone; one with any other
// task is an error. All in one transaction.
func addMiners(ctx context.Context, db *sql.DB, w io.Writer) error {
	devs, err := listDevices(ctx, "ami_mining_controller")
	if err != nil {
		return err
	}
	ms, err := chooseMiners(devs)
	if err != nil {
		return err
	}
	return inTx(ctx, db, func(tx *sql.Tx) error {
		now := time.Now()
		added := 0
		for _, m := range ms {
			t, ok, err := store.TaskOfAssignee(ctx, tx, m.Code)
			if err != nil {
				return err
			}
			if ok {
				if t.Task != mineTask {
					return fmt.Errorf("%s is mining %s, but is assigned task %d (%s at %s)", m.Code, m.Location, t.ID, t.Task, t.Location)
				}
				continue
			}
			id, err := store.AddTask(ctx, tx, mineTask, m.Location, "", store.DefaultPriority, now)
			if err != nil {
				return err
			}
			if err := store.AssignTask(ctx, tx, id, m.Code, now); err != nil {
				return err
			}
			fmt.Fprintf(w, "  new %s %d %s <- %s\n", mineTask, id, m.Location, m.Code)
			added++
		}
		fmt.Fprintf(w, "mine tasks: %d controllers running %s, %d new tasks\n", len(ms), gatherDirective, added)
		return nil
	})
}
