package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

// digestStatus matches the statuses AMI digests report for their devices
// (mining and survey).
var digestStatus = regexp.MustCompile(`^(idle|travelling|scanning|searching|tracking|mining \([a-z]+\))$`)

// ControlledStatus records what an AMI controller's digest says about one of
// its devices: device is controlled by controller and has status (see
// SetStatus). An unknown status, or a device not in the store, is an error.
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
	if err := SetStatus(ctx, ex, device, status); err != nil {
		return err
	}
	return updateOne(ctx, ex, device, `UPDATE devices SET controller = ? WHERE device_code = ?`, controller, device)
}

// SetStatus records device's status. While travel events have it travelling
// (it has a destination), the status goes into status_before_travel instead,
// for travel.arrived to restore; a status of travelling never does.
func SetStatus(ctx context.Context, ex Execer, device, status string) error {
	if status == "" {
		return fmt.Errorf("device %s: empty status", device)
	}
	return updateOne(ctx, ex, device, `UPDATE devices SET
		status_before_travel = CASE
			WHEN status = 'travelling' AND destination IS NOT NULL AND ?1 != 'travelling' THEN ?1
			ELSE status_before_travel END,
		status = CASE WHEN status = 'travelling' AND destination IS NOT NULL THEN status ELSE ?1 END
		WHERE device_code = ?2`, status, device)
}

// Adopted records that controller has adopted device (of deviceType). A
// device of another type, or not in the store, is an error.
func Adopted(ctx context.Context, ex ExecQueryer, controller, device, deviceType string) error {
	if _, err := controlled(ctx, ex, controller, device, deviceType); err != nil {
		return err
	}
	return updateOne(ctx, ex, device, `UPDATE devices SET controller = ? WHERE device_code = ?`, controller, device)
}

// Released records that controller has let go of device (of deviceType). As
// for Adopted, and it's also an error if the store has another controller
// controlling it.
func Released(ctx context.Context, ex ExecQueryer, controller, device, deviceType string) error {
	cur, err := controlled(ctx, ex, controller, device, deviceType)
	if err != nil {
		return err
	}
	if cur != "" && cur != controller {
		return fmt.Errorf("%s released %s, but the store has it controlled by %s", controller, device, cur)
	}
	return updateOne(ctx, ex, device, `UPDATE devices SET controller = NULL WHERE device_code = ?`, device)
}

// controlled checks an adopt or release event and returns device's
// controller as the store has it.
func controlled(ctx context.Context, q Queryer, controller, device, deviceType string) (string, error) {
	if controller == "" || device == "" {
		return "", fmt.Errorf("adopt/release: controller %q, device %q: neither may be empty", controller, device)
	}
	var typ string
	var cur sql.NullString
	err := q.QueryRowContext(ctx, `SELECT device_type, controller FROM devices WHERE device_code = ?`, device).Scan(&typ, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%s adopt/release: device %s not in the store", controller, device)
	}
	if err != nil {
		return "", err
	}
	if typ != deviceType {
		return "", fmt.Errorf("%s adopt/release %s: event says it's a %s, store a %s", controller, device, deviceType, typ)
	}
	return cur.String, nil
}
