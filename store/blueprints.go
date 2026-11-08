package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gcapell/replicant/model"
)

// ReplaceBlueprints replaces the whole blueprints table with a /blueprints
// listing. An empty listing, a device type listed twice or empty, or an
// unknown resource type is an error.
func ReplaceBlueprints(ctx context.Context, ex Execer, all []model.Blueprint) error {
	if len(all) == 0 {
		return errors.New("blueprints: none listed")
	}
	if _, err := ex.ExecContext(ctx, `DELETE FROM blueprints`); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, b := range all {
		if b.DeviceType == "" || seen[b.DeviceType] {
			return fmt.Errorf("blueprints: device type %q listed twice or empty", b.DeviceType)
		}
		seen[b.DeviceType] = true
		if err := putBlueprint(ctx, ex, b); err != nil {
			return fmt.Errorf("blueprint %s: %w", b.DeviceType, err)
		}
	}
	return nil
}

func putBlueprint(ctx context.Context, ex Execer, b model.Blueprint) error {
	for r := range b.Resources {
		if !slices.Contains(resources, string(r)) {
			return fmt.Errorf("unknown resource type %q", r)
		}
	}
	var components any
	if len(b.Components) > 0 {
		j, err := json.Marshal(b.Components) // map keys are sorted
		if err != nil {
			return err
		}
		components = string(j)
	}
	args := []any{b.DeviceType, b.ShortDescription, b.Description,
		null(strings.Join(b.Features, ",")), null(strings.Join(b.Directives, ",")), components,
		b.PrintTime, b.AttachCapacity, b.CargoCapacity, b.StowCapacity, b.QueueSize, b.RelayRange, b.Strength}
	for _, r := range resources {
		args = append(args, b.Resources[model.ResourceType(r)])
	}
	_, err := ex.ExecContext(ctx, fmt.Sprintf(`INSERT INTO blueprints
		(device_type, short_description, description, features, directives, components,
		 print_time, attach_capacity, cargo_capacity, stow_capacity, queue_size, relay_range, strength, %s)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?%s)`,
		strings.Join(resources, ", "), strings.Repeat(", ?", len(resources))), args...)
	return err
}
