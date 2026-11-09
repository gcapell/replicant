package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// fusionBarge is the cost and print time in a real blueprint.unlocked.
var fusionBarge = map[model.ResourceType]int{"conductive": 50, "rares": 150, "silicates": 40, "structural": 240, "volatiles": 110}

// fakeBlueprints makes /blueprints answer with list.
func fakeBlueprints(t *testing.T, list ...model.Blueprint) {
	fetchBlueprints = func(context.Context) (*model.Blueprints, error) {
		return &model.Blueprints{Blueprints: list}, nil
	}
	t.Cleanup(func() { fetchBlueprints = api.AllBlueprints })
}

func unlockEvent(id string) received {
	e := ev(id, "blueprint.unlocked")
	e.ReplicantCode = "4698E8E6"
	return received{e, &model.BlueprintUnlocked{DeviceType: "fusion_barge", PrintTime: 1800, Resources: fusionBarge, Source: "achievement"}}
}

func TestUnlockedBlueprint(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	old := model.Blueprint{DeviceType: "ftl_relay", PrintTime: 600, Resources: map[model.ResourceType]int{"rares": 10}}
	if err := store.ReplaceBlueprints(ctx, db, []model.Blueprint{old}); err != nil {
		t.Fatal(err)
	}
	barge := model.Blueprint{DeviceType: "fusion_barge", PrintTime: 1800, Resources: fusionBarge, AttachCapacity: 2}

	// Each of these stops monitor and leaves the event unrecorded.
	slower := barge
	slower.PrintTime = 1900
	dearer := barge
	dearer.Resources = map[model.ResourceType]int{"rares": 1}
	for name, list := range map[string][]model.Blueprint{
		"not listed":         {old},
		"another print time": {old, slower},
		"another cost":       {old, dearer},
	} {
		fakeBlueprints(t, list...)
		if err := recordAll(ctx, db, feed(unlockEvent("700-0")), &printer{w: &strings.Builder{}}, false); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if n, _ := store.Count(ctx, db, "events"); n != 0 {
		t.Errorf("%d events recorded, want 0", n)
	}

	fakeBlueprints(t, old, barge)
	if err := recordAll(ctx, db, feed(unlockEvent("700-0")), &printer{w: &strings.Builder{}}, false); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Count(ctx, db, "blueprints"); n != 2 {
		t.Errorf("%d blueprints, want 2", n)
	}
	var attach, rares int
	if err := db.QueryRow(`SELECT attach_capacity, rares FROM blueprints WHERE device_type = 'fusion_barge'`).Scan(&attach, &rares); err != nil ||
		attach != 2 || rares != 150 {
		t.Errorf("fusion_barge: attach %d, rares %d, %v", attach, rares, err)
	}
}
