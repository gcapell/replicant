package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// handler works out what an event changes. It may read the store and call
// the API, but mustn't write: it returns the change, which recordAndApply
// makes in the transaction that records the event. So everything an event
// needs is fetched before that transaction begins.
type handler func(ctx context.Context, db store.Reader, env api.Envelope, payload any) (change, error)

// change applies an event to the store, inside the transaction that records it.
type change func(ctx context.Context, tx *sql.Tx) error

// direct is a handler for an event that needs nothing but itself.
func direct(apply func(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error) handler {
	return func(_ context.Context, _ store.Reader, env api.Envelope, payload any) (change, error) {
		return func(ctx context.Context, tx *sql.Tx) error { return apply(ctx, tx, env, payload) }, nil
	}
}

// handlers are the events we apply. Every other event is recorded but not
// applied (for now).
var handlers = map[string]handler{
	"travel.departed": direct(departed),
	"travel.arrived":  direct(arrived),
	"print.completed": printed,
	"device.attached": direct(attached),
	"device.detached": direct(detached),
	"device.deployed": deployed,
	"relay.activated": direct(relayActivated),

	"device.compacting": direct(compacting),
	"device.compacted":  direct(compacted),
	"device.unfurling":  direct(unfurling),
	"device.unfurled":   unfurled,

	"ami.mining.digest": direct(miningDigest),
	"ami.survey.digest": direct(surveyDigest),
	"ami.adopted":       direct(adopted),
	"ami.released":      direct(released),

	"directive.set":       directive,
	"directive.completed": directive,
	"directive.cleared":   directive,

	"prospect.completed": prospected,

	"experience.gained": experienced,

	"blueprint.unlocked": unlockedBlueprint,
}

// surveyDigest is miningDigest for an ami_survey_controller.
func surveyDigest(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.SurveyDigest)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return digestDevices(ctx, tx, env, p.Devices)
}

// adopted: the envelope's device is the controller, adopting each of
// devices.
func adopted(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.AMIAdopted)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	for _, d := range p.Devices {
		if err := store.Adopted(ctx, tx, env.DeviceCode, d.DeviceCode, d.DeviceType); err != nil {
			return err
		}
	}
	return nil
}

// released: the envelope's device is the controller, letting go of devices.
func released(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.AMIReleased)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	for _, d := range p.Devices {
		if err := store.Released(ctx, tx, env.DeviceCode, d.DeviceCode, d.DeviceType); err != nil {
			return err
		}
	}
	return nil
}

// directive handles directive.set, .completed and .cleared. None says what
// the device's status becomes (coordinating, idle, ...), so it's fetched from
// /devices/{code}: when replaying old events that's the status now, not then.
// A device the API doesn't know, or of a different type, is an error.
func directive(ctx context.Context, _ store.Reader, env api.Envelope, _ any) (change, error) {
	d, err := eventDevice(ctx, env)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx *sql.Tx) error {
		return store.SetStatus(ctx, tx, d.DeviceCode, d.Status)
	}, nil
}

// miningDigest records the status of each device an ami_mining_controller
// (the envelope's device) reports, and that the controller controls it.
func miningDigest(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.MiningDigest)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return digestDevices(ctx, tx, env, p.Devices)
}

// digestDevices records the status of each device a controller's digest
// lists, and that the controller (the envelope's device) controls it.
func digestDevices(ctx context.Context, tx *sql.Tx, env api.Envelope, devices []model.AMIDevice) error {
	for _, d := range devices {
		if err := store.ControlledStatus(ctx, tx, env.DeviceCode, d.DeviceCode, d.Status); err != nil {
			return err
		}
	}
	return nil
}

// attached and detached: the envelope's device is the carrier, and its
// location is where both are. There's one event per attached device.
func attached(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.DeviceAttached)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return store.Attached(ctx, tx, env.DeviceCode, env.Location, p.TargetCode, p.TargetType)
}

func detached(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.DeviceDetached)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	return store.Detached(ctx, tx, env.DeviceCode, env.Location, p.TargetCode, p.TargetType)
}

// fetchDevice is api.Device, replaceable in tests.
var fetchDevice = api.Device

// eventDevice fetches the envelope's device from /devices/{code}. A device
// the API doesn't know, or of a different type, is an error.
func eventDevice(ctx context.Context, env api.Envelope) (*model.Device, error) {
	if env.DeviceCode == "" {
		return nil, fmt.Errorf("no device_code")
	}
	d, err := fetchDevice(ctx, env.DeviceCode)
	if err != nil {
		return nil, err
	}
	if d.DeviceCode != env.DeviceCode || d.DeviceType != env.DeviceType {
		return nil, fmt.Errorf("event is about %s %s, but /devices/%s is %s %s",
			env.DeviceType, env.DeviceCode, env.DeviceCode, d.DeviceType, d.DeviceCode)
	}
	return d, nil
}

// printed adds a newly printed device to the store, as /devices/{code}
// describes it now (when replaying old events, that may be after it has moved
// on). A device that the API doesn't know, or of a different type from the
// one printed, is an error.
func printed(ctx context.Context, _ store.Reader, env api.Envelope, payload any) (change, error) {
	p, ok := payload.(*model.PrintCompleted)
	if !ok {
		return nil, fmt.Errorf("payload is %T", payload)
	}
	if p.NewDeviceCode == "" {
		return nil, fmt.Errorf("no new_device_code")
	}
	d, err := fetchDevice(ctx, p.NewDeviceCode)
	if err != nil {
		return nil, err
	}
	if d.DeviceCode != p.NewDeviceCode || d.DeviceType != p.DeviceType {
		return nil, fmt.Errorf("printed a %s %s, but /devices/%s is a %s %s",
			p.DeviceType, p.NewDeviceCode, p.NewDeviceCode, d.DeviceType, d.DeviceCode)
	}
	return func(ctx context.Context, tx *sql.Tx) error { return store.PutDevice(ctx, tx, *d) }, nil
}

func departed(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.TravelDeparted)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	arrives, err := departureArrival(env, p)
	if err != nil {
		return err
	}
	return store.Departed(ctx, tx, env.DeviceCode, p.Destination, arrives, p.AttachedDevices)
}

// departureArrival is when a departing device is due: arrives_at if the event
// has it, otherwise its creation time plus travel_time_seconds.
func departureArrival(env api.Envelope, p *model.TravelDeparted) (time.Time, error) {
	if p.ArrivesAt != "" {
		return time.Parse(time.RFC3339, p.ArrivesAt)
	}
	if p.TravelTimeSeconds <= 0 || env.CreatedAt.IsZero() {
		return time.Time{}, fmt.Errorf("no arrives_at, and travel_time_seconds %v", p.TravelTimeSeconds)
	}
	return env.CreatedAt.Add(time.Duration(p.TravelTimeSeconds * float64(time.Second))), nil
}

func arrived(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.TravelArrived)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	// After a surge the payload's destination is the bare star; the
	// envelope's location is where the device actually is.
	return store.Arrived(ctx, tx, env.DeviceCode, env.Location, p.AttachedDevices)
}
