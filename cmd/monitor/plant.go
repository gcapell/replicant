package main

// plant_relay tasks: plant an ftl_relay at a star (the task's location, a
// bare star) that can join the relay mesh. They're filed on every sweep and
// whenever a prospect finds new stars (filePlantRelays). Nothing carries
// them out yet: the handler only logs, and no device type is given them.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

const plantRelayTask = "plant_relay"

// plantRelay is the plant_relay task's handler: not implemented yet.
func plantRelay(_ context.Context, _ store.Reader, t store.Task) (taskChange, error) {
	log.Printf("task %d %s %s (%s): not implemented yet", t.ID, t.Task, t.Location, t.Assignee)
	return nil, nil
}

// newPlantRelays lists the stars that can join the mesh (auto.Connectable:
// within reach of a relay star, or of another star that can), in the order
// the mesh would grow, leaving out those that already have a plant_relay
// task (of any status) in hasTask.
func newPlantRelays(stars []model.Star, relay []string, hasTask map[string]bool) []string {
	isRelay := map[string]bool{}
	for _, r := range relay {
		isRelay[r] = true
	}
	var mesh, cands []model.Star
	for _, s := range stars {
		if isRelay[s.Designation] {
			mesh = append(mesh, s)
		} else {
			cands = append(cands, s)
		}
	}
	var out []string
	for _, s := range auto.Connectable(mesh, cands, func(s model.Star) model.Position { return s.Position }) {
		if !hasTask[s.Designation] {
			out = append(out, s.Designation)
		}
	}
	return out
}

// filePlantRelays files a pending plant_relay task for each star that can
// join the mesh and has none yet (newPlantRelays).
func filePlantRelays(ctx context.Context, tx *sql.Tx, w io.Writer) error {
	stars, err := store.Stars(ctx, tx)
	if err != nil {
		return err
	}
	relay, err := store.RelayStars(ctx, tx)
	if err != nil {
		return err
	}
	existing, err := store.TasksOfType(ctx, tx, plantRelayTask)
	if err != nil {
		return err
	}
	hasTask := map[string]bool{}
	for _, t := range existing {
		hasTask[t.Location] = true
	}
	now := time.Now()
	for _, s := range newPlantRelays(stars, relay, hasTask) {
		id, err := store.AddTask(ctx, tx, plantRelayTask, s, "", store.DefaultPriority, now)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  new %s %d %s\n", plantRelayTask, id, s)
	}
	return nil
}
