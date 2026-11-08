package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gcapell/replicant/model"
)

var printTime = map[string]time.Duration{
	"autofactory":     40 * time.Minute,
	"cargo_freighter": 20 * time.Minute,
	"mining_drone":    10 * time.Minute,
}

func TestScheduleChainsQueueAfterCurrentPrint(t *testing.T) {
	now := time.Date(2026, 11, 8, 13, 37, 0, 0, time.Local)
	d := &model.Device{
		DeviceCode: "072FB5E6", Location: "OTHONAL-BELT-1", Status: "printing (autofactory)",
		Printing: model.PrintingStatus{
			DeviceType:    "autofactory",
			PendingStatus: model.PendingStatus{CompletesAt: now.Add(2 * time.Hour).UTC().Format(time.RFC3339)},
		},
		PrintQueue: []model.DevicePrintQueueItem{{DeviceType: "cargo_freighter"}, {DeviceType: "mining_drone"}},
	}
	f, err := schedule(d, printTime, now)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	report(&b, []Factory{f}, now)
	want := `OTHONAL-BELT-1
   072FB5E6
       autofactory (2h ETA 15:37)
       cargo_freighter (20m ETA 15:57)
       mining_drone (10m ETA 16:07)
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestScheduleNotPrintingHasNoETAs(t *testing.T) {
	now := time.Date(2026, 11, 8, 13, 37, 0, 0, time.Local)
	d := &model.Device{
		DeviceCode: "AAAA0001", Location: "MIRETH-BELT-2", Status: "waiting_for_resources",
		WaitingFor: map[model.ResourceType]model.WaitingForStat{
			model.Silicates: {Have: 0, Need: 20}, model.Carbon: {Have: 10, Need: 50},
		},
		PrintQueue: []model.DevicePrintQueueItem{{DeviceType: "cargo_freighter"}},
	}
	idle := &model.Device{DeviceCode: "AAAA0000", Location: "MIRETH-BELT-2", Status: "idle"}
	var fs []Factory
	for _, d := range []*model.Device{d, idle} {
		f, err := schedule(d, printTime, now)
		if err != nil {
			t.Fatal(err)
		}
		fs = append(fs, f)
	}
	var b strings.Builder
	report(&b, fs, now)
	want := `MIRETH-BELT-2
   AAAA0000
       (nothing queued)
   AAAA0001 waiting_for_resources waiting for C 10/50 Si 0/20
       cargo_freighter (20m)
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestScheduleUnknownBlueprint(t *testing.T) {
	d := &model.Device{DeviceCode: "X", PrintQueue: []model.DevicePrintQueueItem{{DeviceType: "warp_core"}}}
	if _, err := schedule(d, printTime, time.Now()); err == nil {
		t.Error("want an error for a queued type with no blueprint")
	}
}

func TestClockShowsDayWhenNotToday(t *testing.T) {
	now := time.Date(2026, 11, 8, 23, 0, 0, 0, time.Local)
	if got := clock(now.Add(2*time.Hour), now); got != "Mon 01:00" {
		t.Errorf("clock = %q", got)
	}
}

func TestDur(t *testing.T) {
	for d, want := range map[time.Duration]string{
		45 * time.Second: "45s", 20 * time.Minute: "20m", 2 * time.Hour: "2h", 90 * time.Minute: "1h30m",
	} {
		if got := dur(d); got != want {
			t.Errorf("dur(%v) = %q, want %q", d, got, want)
		}
	}
}
