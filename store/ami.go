package store

import (
	"context"
	"fmt"
	"regexp"
)

// digestStatus matches the statuses AMI digests report for their devices.
var digestStatus = regexp.MustCompile(`^(idle|travelling|mining \([a-z]+\))$`)

// ControlledStatus records what an AMI controller's digest says about one of
// its devices: device is controlled by controller and has status. While the
// store has the device travelling (from travel events) the status is kept in
// status_before_travel, for travel.arrived to restore. An unknown status, or
// a device not in the store, is an error.
//
// A digest saying travelling doesn't move the device: AMI drones' trips
// (e.g. to a salvage site) mostly don't reach the stream as travel events, so
// its location stays as it was.
func ControlledStatus(ctx context.Context, ex Execer, controller, device, status string) error {
	if controller == "" || device == "" {
		return fmt.Errorf("digest: controller %q, device %q: neither may be empty", controller, device)
	}
	if !digestStatus.MatchString(status) {
		return fmt.Errorf("digest from %s: device %s has unknown status %q", controller, device, status)
	}
	// While travel events have it travelling (it has a destination), keep the
	// status for arrival.
	if err := updateOne(ctx, ex, device, `UPDATE devices SET
		status_before_travel = CASE
			WHEN status = 'travelling' AND destination IS NOT NULL AND ?1 != 'travelling' THEN ?1
			ELSE status_before_travel END,
		status = CASE WHEN status = 'travelling' AND destination IS NOT NULL THEN status ELSE ?1 END
		WHERE device_code = ?2`, status, device); err != nil {
		return err
	}
	return updateOne(ctx, ex, device, `UPDATE devices SET controller = ? WHERE device_code = ?`, controller, device)
}
