package main

import (
	"strings"
	"testing"

	"github.com/gcapell/replicant/model"
)

func TestTransportMismatch(t *testing.T) {
	st := transport("T1", "A-1", 2)
	pickup := obsPlan{Obs: model.Device{DeviceCode: "O1"}, Transport: "T1", Steps: []step{{Kind: attach}}}
	carry := obsPlan{Obs: model.Device{DeviceCode: "O1"}, Transport: "T1", Steps: []step{{Kind: detach}}}

	moved := st
	moved.Location, moved.Status = "", "travelling"
	carrying := st
	carrying.AttachedDevices = []model.AttachedDevice{{DeviceCode: "X"}}
	away := st
	away.Status, away.Location, away.Travel.FinalDestination = "travelling", "", "B-1-L4"
	elsewhere := away
	elsewhere.Travel.FinalDestination = "C-1-L4"

	for _, tt := range []struct {
		name     string
		p        obsPlan
		st, api  model.Device
		problems []string
	}{
		{"same", pickup, st, st, nil},
		{"moved", pickup, st, moved, []string{"location", "status"}},
		{"now carrying", pickup, st, carrying, []string{"already carrying"}},
		{"carrying our observatory is fine", carry, st, carrying, nil},
		{"travelling elsewhere", carry, away, elsewhere, []string{"destination"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			errs := transportMismatch(tt.p, tt.st, tt.api)
			if len(errs) != len(tt.problems) {
				t.Fatalf("got %v, want %d problems like %q", errs, len(tt.problems), tt.problems)
			}
			for i, want := range tt.problems {
				if !strings.Contains(errs[i].Error(), want) {
					t.Errorf("problem %d %q, want it to mention %q", i, errs[i], want)
				}
			}
		})
	}
}
