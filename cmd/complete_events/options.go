package main

import "github.com/gcapell/replicant/model"

// chooseOption picks which of an event's alternative options to work on: a
// satisfied one (everything it asks for is there) if there is one, otherwise
// the first whose missing devices can all come from spare (device type ->
// count), otherwise the first. The
// chosen option's missing devices are taken off spare, so later events
// don't count on the same devices.
//
// FIXME: resources don't affect the choice.
func chooseOption(ev *model.LocationEvent, spare map[string]int) int {
	opts := ev.Progress.Options
	for i, o := range opts {
		if o.Satisfied() {
			return i
		}
	}
	choice := 0
	for i, o := range opts {
		if devicesAvailable(o, spare) {
			choice = i
			break
		}
	}
	if choice < len(opts) {
		for _, d := range opts[choice].Devices {
			if left := d.Required - d.Current; left > 0 {
				spare[d.DeviceType] = max(0, spare[d.DeviceType]-left)
			}
		}
	}
	return choice
}

func devicesAvailable(o model.EventOption, spare map[string]int) bool {
	for _, d := range o.Devices {
		if d.Required-d.Current > spare[d.DeviceType] {
			return false
		}
	}
	return true
}
