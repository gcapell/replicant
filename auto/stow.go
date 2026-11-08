package auto

import (
	"context"
	"fmt"
	"log"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
)

// StowInto stows device (owned by replicant owner) in host, whose replicant
// is hostReplicant. A device can only be stowed in a host belonging to its
// own replicant, so it hands the device over first if needed. Both must be at
// the same location. Stow is immediate: no error means it's in the hold.
func StowInto(ctx context.Context, device, owner, host, hostReplicant string) error {
	if owner != hostReplicant {
		if err := api.ChangeOwner(ctx, device, hostReplicant); err != nil {
			return fmt.Errorf("change owner of %s %s -> %s: %w", device, owner, hostReplicant, err)
		}
		log.Printf("%s now owned by %s (was %s)", device, hostReplicant, owner)
	}
	if err := api.Stow(ctx, device, host); err != nil {
		return fmt.Errorf("stow %s in %s: %w", device, host, err)
	}
	log.Printf("stowed %s in %s", device, host)
	return nil
}

// ReadyReplicant reports whether d is an idle heaven_vessel (not attached or
// stowed) carrying a replicant_matrix.
func ReadyReplicant(d model.Device) bool {
	if d.Status != "idle" || d.AttachedToDeviceCode != "" || d.StowedInDeviceCode != "" {
		return false
	}
	for _, s := range d.StowedDevices {
		if s.DeviceType == "replicant_matrix" {
			return true
		}
	}
	return false
}
