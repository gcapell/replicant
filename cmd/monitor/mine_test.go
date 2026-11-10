package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

func controller(code, loc, directive string) model.Device {
	d := model.Device{DeviceCode: code, DeviceType: "ami_mining_controller", Location: loc, Status: "coordinating"}
	d.AmiDirective.Name = directive
	if directive != "" {
		d.AmiDirectiveStatus = "active"
	}
	return d
}

func TestAddMiners(t *testing.T) {
	db := taskStore(t)
	ctx := t.Context()
	devs := []model.Device{
		controller("M1", "OTHYONAL-BELT-1", gatherDirective),
		controller("M2", "WYNUS-BELT-1", gatherDirective),
		controller("IDLE", "OTHYONAL-BELT-1", ""),
		controller("SALVAGER", "RILKRDANA-1", "gather_salvage"),
	}
	listDevices = func(_ context.Context, typ string) ([]model.Device, error) {
		if typ != "ami_mining_controller" {
			t.Errorf("listed %s", typ)
		}
		return devs, nil
	}
	defer func() { listDevices = auto.Devices }()
	// M2 already has its task.
	exec(t, db, `INSERT INTO tasks (id, task, location, status, assignee, created_at, updated_at)
		VALUES (1, 'mine', 'WYNUS-BELT-1', 'assigned', 'M2', 0, 0)`)

	var out strings.Builder
	if err := addMiners(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	want := []string{"1 mine WYNUS-BELT-1 assigned M2", "2 mine OTHYONAL-BELT-1 assigned M1"}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q\n%s", got, want, out.String())
	}
	// Again: nothing new.
	if err := addMiners(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	if got := taskRows(t, db, "tasks"); !slices.Equal(got, want) {
		t.Errorf("second run: tasks = %q", got)
	}
	// The handler does nothing, and running every assigned task is fine.
	if err := runAssigned(ctx, db); err != nil {
		t.Fatal(err)
	}

	// A miner with another task, or mining somewhere other than a belt, stops it.
	exec(t, db, `INSERT INTO tasks (task, location, status, assignee, created_at, updated_at)
		VALUES ('survey_system', 'FAR', 'assigned', 'M3', 0, 0)`)
	devs = []model.Device{controller("M3", "FAR-BELT-1", gatherDirective)}
	if err := addMiners(ctx, db, &out); err == nil || !strings.Contains(err.Error(), "survey_system") {
		t.Errorf("miner with another task: err = %v", err)
	}
	devs = []model.Device{controller("M4", "FAR-3", gatherDirective)}
	if err := addMiners(ctx, db, &out); err == nil || !strings.Contains(err.Error(), "not at a belt") {
		t.Errorf("miner off a belt: err = %v", err)
	}
	devs = []model.Device{{DeviceCode: "X", DeviceType: "survey_drone"}}
	if err := addMiners(ctx, db, &out); err == nil {
		t.Error("wrong device type accepted")
	}
}
