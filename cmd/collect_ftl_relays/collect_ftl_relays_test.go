package main

// Run with a dummy key, e.g. API_KEY=x go test ./cmd/collect_ftl_relays (api's init exits without one).

import (
	"slices"
	"testing"

	"github.com/gcapell/replicant/model"
)

func TestIdleRelaysAt(t *testing.T) {
	devices := []model.Device{
		{DeviceCode: "C", DeviceType: "ftl_relay", Status: "inactive", Location: "SOL-4-L4"},
		{DeviceCode: "A", DeviceType: "ftl_relay", Status: "inactive", Location: "SOL-4-L4"},
		{DeviceCode: "R", DeviceType: "ftl_relay", Status: "relaying", Location: "SOL-4-L4"},                          // in use
		{DeviceCode: "E", DeviceType: "ftl_relay", Status: "inactive", Location: "SOL-4-L5"},                          // elsewhere in the star
		{DeviceCode: "S", DeviceType: "ftl_relay", Status: "inactive", Location: "SOL-4-L4", StowedInDeviceCode: "V"}, // already stowed
		{DeviceCode: "B", DeviceType: "ftl_beacon", Status: "inactive", Location: "SOL-4-L4"},                         // not a relay
	}
	var got []string
	for _, d := range inactiveRelaysAt(devices, "SOL-4-L4") {
		got = append(got, d.DeviceCode)
	}
	slices.Sort(got) // order doesn't matter
	if want := []string{"A", "C"}; !slices.Equal(got, want) {
		t.Errorf("inactiveRelaysAt = %v, want %v", got, want)
	}
}
