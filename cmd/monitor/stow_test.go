package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestDeployAndActivate(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	const vessel, relay, other = "BA7BB196", "4A71C3A1", "6F560821"
	for _, d := range []model.Device{
		{DeviceCode: vessel, DeviceType: "heaven_vessel", Status: "idle", Location: "KYRZHBIPOS-7-L4"},
		{DeviceCode: relay, DeviceType: "ftl_relay", Status: "stowed", StowedInDeviceCode: vessel},
		{DeviceCode: other, DeviceType: "ftl_relay", Status: "stowed", StowedInDeviceCode: "AA2CEC89"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		switch code {
		case relay, other:
			return &model.Device{DeviceCode: code, DeviceType: "ftl_relay", Status: "inactive"}, nil
		}
		return nil, errors.New("404 not found")
	}
	defer func() { fetchDevice = api.Device }()

	n := 0
	send := func(name, code, typ, loc string, p any) error {
		n++
		e := ev(strings.Repeat("9", n)+"-0", name)
		e.DeviceCode, e.DeviceType, e.Location = code, typ, loc
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}
	check := func(when, status, loc string, stowed bool) {
		t.Helper()
		var s string
		var l, in sql.NullString
		if err := db.QueryRow(`SELECT status, location, stowed_in FROM devices WHERE device_code = ?`, relay).
			Scan(&s, &l, &in); err != nil {
			t.Fatal(err)
		}
		if s != status || l.String != loc || in.Valid != stowed {
			t.Errorf("%s: status %q, location %q, stowed_in %v; want %q, %q, stowed %v", when, s, l.String, in, status, loc, stowed)
		}
	}
	relayStars := func() []string {
		t.Helper()
		var stars []string
		rows, err := db.Query(`SELECT star FROM relay_stars`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			rows.Scan(&s)
			stars = append(stars, s)
		}
		return stars
	}

	const at = "KYRZHBIPOS-7-L4"
	if err := send("relay.activated", relay, "ftl_relay", at, &model.RelayActivated{Location: at, Star: "KYRZHBIPOS"}); err == nil {
		t.Errorf("activated while stowed: accepted")
	}
	if err := send("device.deployed", relay, "ftl_relay", at, &model.DeviceDeployed{DeployedFrom: vessel}); err != nil {
		t.Fatal(err)
	}
	check("deployed", "inactive", at, false)
	// Again, as when the snapshot is newer than the event.
	if err := send("device.deployed", relay, "ftl_relay", at, &model.DeviceDeployed{DeployedFrom: vessel}); err != nil {
		t.Errorf("deployed again: %v", err)
	}
	if err := send("relay.activated", relay, "ftl_relay", at, &model.RelayActivated{Location: at, Star: "KYRZHBIPOS"}); err != nil {
		t.Fatal(err)
	}
	check("activated", "relaying", at, false)
	if got := relayStars(); len(got) != 1 || got[0] != "KYRZHBIPOS" {
		t.Errorf("relay_stars = %v", got)
	}

	for _, c := range []struct {
		what, name, code, typ, loc string
		p                          any
	}{
		{"no deployed_from", "device.deployed", other, "ftl_relay", at, &model.DeviceDeployed{}},
		{"stowed elsewhere", "device.deployed", other, "ftl_relay", at, &model.DeviceDeployed{DeployedFrom: vessel}},
		{"deployed elsewhere", "device.deployed", relay, "ftl_relay", "ALKANOTH-3-L4", &model.DeviceDeployed{DeployedFrom: vessel}},
		{"wrong type", "device.deployed", relay, "heaven_vessel", at, &model.DeviceDeployed{DeployedFrom: vessel}},
		{"unknown to the API", "device.deployed", vessel, "heaven_vessel", at, &model.DeviceDeployed{DeployedFrom: "X"}},
		{"payload elsewhere", "relay.activated", relay, "ftl_relay", at, &model.RelayActivated{Location: "ALKANOTH-3-L4", Star: "ALKANOTH"}},
		{"wrong star", "relay.activated", relay, "ftl_relay", at, &model.RelayActivated{Location: at, Star: "ALKANOTH"}},
		{"not where the store has it", "relay.activated", relay, "ftl_relay", "ALKANOTH-3-L4", &model.RelayActivated{Location: "ALKANOTH-3-L4", Star: "ALKANOTH"}},
		{"stowed", "relay.activated", other, "ftl_relay", at, &model.RelayActivated{Location: at, Star: "KYRZHBIPOS"}},
		{"not a relay", "relay.activated", vessel, "heaven_vessel", at, &model.RelayActivated{Location: at, Star: "KYRZHBIPOS"}},
		{"not in the store", "relay.activated", "DEADBEEF", "ftl_relay", at, &model.RelayActivated{Location: at, Star: "KYRZHBIPOS"}},
	} {
		if err := send(c.name, c.code, c.typ, c.loc, c.p); err == nil {
			t.Errorf("%s: accepted", c.what)
		}
	}
	check("after the failures", "relaying", at, false)
}
