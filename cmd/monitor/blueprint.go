package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"maps"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// fetchBlueprints is api.AllBlueprints, replaceable in tests.
var fetchBlueprints = api.AllBlueprints

// unlockedBlueprint handles blueprint.unlocked. The payload has the new
// device type's cost and print time, but not its features, directives or
// capacities, so we read /blueprints again and replace the whole table. It's
// an error if the listing doesn't include the unlocked type, or disagrees
// with the event about its print time or cost.
func unlockedBlueprint(ctx context.Context, _ store.Reader, _ api.Envelope, payload any) (change, error) {
	p, ok := payload.(*model.BlueprintUnlocked)
	if !ok {
		return nil, fmt.Errorf("payload is %T", payload)
	}
	if p.DeviceType == "" {
		return nil, fmt.Errorf("no device_type")
	}
	bs, err := fetchBlueprints(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkUnlocked(p, bs.Blueprints); err != nil {
		return nil, err
	}
	log.Printf("blueprint unlocked (%s): %s; %d device types", p.Source, p.DeviceType, len(bs.Blueprints))
	return func(ctx context.Context, tx *sql.Tx) error {
		return store.ReplaceBlueprints(ctx, tx, bs.Blueprints)
	}, nil
}

// checkUnlocked checks that all lists the unlocked blueprint, agreeing with
// the event about its print time and cost.
func checkUnlocked(p *model.BlueprintUnlocked, all []model.Blueprint) error {
	for _, b := range all {
		if b.DeviceType != p.DeviceType {
			continue
		}
		if b.PrintTime != p.PrintTime {
			return fmt.Errorf("blueprint %s unlocked with print_time %d, but /blueprints says %d", p.DeviceType, p.PrintTime, b.PrintTime)
		}
		if !maps.Equal(b.Resources, p.Resources) {
			return fmt.Errorf("blueprint %s unlocked costing %v, but /blueprints says %v", p.DeviceType, p.Resources, b.Resources)
		}
		return nil
	}
	return fmt.Errorf("blueprint %s unlocked, but /blueprints doesn't list it", p.DeviceType)
}
