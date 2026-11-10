package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// prospectStore has two placed replicants (NEAR at CORBRENEUS, FAR at
// CORVALEX) and a travelling one (MOVING), and the stars they're at.
func prospectStore(t *testing.T) *sql.DB {
	t.Helper()
	db := openStore(t)
	ctx := t.Context()
	for _, s := range []model.Star{
		{Designation: "INTERRANAE", Position: model.Position{X: 0}},
		{Designation: "CORBRENEUS", Position: model.Position{X: 3}},
		{Designation: "CORVALEX", Position: model.Position{X: 10}},
	} {
		if err := store.PutStar(ctx, db, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []model.Device{
		{DeviceCode: "VFAR0001", DeviceType: "heaven_vessel", ReplicantCode: "FAR", Status: "idle", Location: "CORVALEX-3"},
		{DeviceCode: "MFAR0001", DeviceType: "replicant_matrix", Status: "stowed", StowedInDeviceCode: "VFAR0001"},
		{DeviceCode: "VNEAR001", DeviceType: "heaven_vessel", ReplicantCode: "NEAR", Status: "idle", Location: "CORBRENEUS-2"},
		{DeviceCode: "MNEAR001", DeviceType: "replicant_matrix", Status: "stowed", StowedInDeviceCode: "VNEAR001"},
		{DeviceCode: "VMOVE001", DeviceType: "heaven_vessel", ReplicantCode: "MOVING", Status: "travelling"},
		{DeviceCode: "MMOVE001", DeviceType: "replicant_matrix", Status: "stowed", StowedInDeviceCode: "VMOVE001"},
		{DeviceCode: "68D1D1CA", DeviceType: "galactic_observatory", Status: "prospecting", Location: "INTERRANAE-3-L4"},
		{DeviceCode: "RINT0001", DeviceType: "ftl_relay", Status: "relaying", Location: "INTERRANAE-3-L4"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	exec(t, db, `INSERT INTO observations (star, dir_x, dir_y, dir_z, device_code, status, created_at, updated_at)
		VALUES ('INTERRANAE', 1, 0, 0, '68D1D1CA', 'prospecting', 0, 0)`)
	// The observatory reports idle once it's done.
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		return &model.Device{DeviceCode: code, DeviceType: "galactic_observatory", Status: "idle", Location: "INTERRANAE-3-L4"}, nil
	}
	t.Cleanup(func() { fetchDevice = api.Device })
	return db
}

// fakeStars answers for STAR1 ... STARn, as seen from replicant NEAR,
// placing each STARi at x=i, and records the stars asked for.
func fakeStars(t *testing.T, n int) *[]string {
	var asked []string
	fetchStar = func(ctx context.Context, replicant, star string) (*model.Star, error) {
		if replicant != "NEAR" {
			return nil, fmt.Errorf("asked replicant %s", replicant)
		}
		asked = append(asked, star)
		var i int
		if _, err := fmt.Sscanf(star, "STAR%d", &i); err != nil || i < 1 || i > n {
			return nil, errors.New("404 not found")
		}
		return &model.Star{Designation: star, EntryPoint: star + "-1-L4", Position: model.Position{X: float64(i)}}, nil
	}
	t.Cleanup(func() { fetchStar = api.StarNear })
	return &asked
}

func prospectEvent(id string, stars ...string) received {
	e := ev(id, "prospect.completed")
	e.DeviceCode, e.DeviceType, e.Star, e.Location = "68D1D1CA", "galactic_observatory", "INTERRANAE", "INTERRANAE-3-L4"
	return received{e, &model.ProspectCompleted{Origin: "INTERRANAE", Stars: stars, StarsGenerated: len(stars)}}
}

func TestProspected(t *testing.T) {
	db := prospectStore(t)
	ctx := t.Context()
	asked := fakeStars(t, 1000)

	err := recordAll(ctx, db, feed(prospectEvent("600-0", "STAR7", "STAR120")), &printer{w: &strings.Builder{}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(*asked) != "[STAR7 STAR120]" {
		t.Errorf("stars asked for: %v", *asked)
	}
	var x float64
	var entry string
	if err := db.QueryRow(`SELECT x, entry_point FROM stars WHERE designation = 'STAR120'`).Scan(&x, &entry); err != nil ||
		x != 120 || entry != "STAR120-1-L4" {
		t.Errorf("STAR120: %v %q %v", x, entry, err)
	}
	if n, _ := store.Count(ctx, db, "stars"); n != 5 {
		t.Errorf("%d stars, want 5", n)
	}
	var status, found, devStatus string
	db.QueryRow(`SELECT status, stars_found FROM observations WHERE star = 'INTERRANAE'`).Scan(&status, &found)
	if status != "success" || found != "STAR7,STAR120" {
		t.Errorf("observation: %s %q, want success STAR7,STAR120", status, found)
	}
	db.QueryRow(`SELECT status FROM devices WHERE device_code = '68D1D1CA'`).Scan(&devStatus)
	if devStatus != "idle" {
		t.Errorf("observatory status %q, want idle", devStatus)
	}
	// From the relay at INTERRANAE (x=0): CORBRENEUS (3), STAR7 (4 on from
	// there), CORVALEX (3 on from STAR7). STAR120 is out of reach.
	if got, want := taskRows(t, db, "tasks"), []string{
		"1 plant_relay CORBRENEUS pending", "2 plant_relay STAR7 pending", "3 plant_relay CORVALEX pending",
	}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}

	// Each of these stops monitor and leaves the event unrecorded.
	unknown := prospectEvent("601-0", "STAR1", "STAR1001")
	mismatch := prospectEvent("602-0", "STAR1")
	mismatch.payload.(*model.ProspectCompleted).StarsGenerated = 2
	elsewhere := prospectEvent("603-0", "STAR1")
	elsewhere.env.Star = "CORVALEX"
	again := prospectEvent("604-0", "STAR1") // already done, with other stars
	notObs := prospectEvent("605-0", "STAR1")
	notObs.env.DeviceType = "ftl_relay"
	for name, r := range map[string]received{
		"unknown star":                    unknown,
		"stars_generated is off":          mismatch,
		"origin isn't the star":           elsewhere,
		"no observation prospecting":      again,
		"not from a galactic_observatory": notObs,
	} {
		if err := recordAll(ctx, db, feed(r), &printer{w: &strings.Builder{}}, false); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if last, _ := store.LastEventID(ctx, db); last != "600-0" {
		t.Errorf("last recorded %q, want 600-0", last)
	}
	if n, _ := store.Count(ctx, db, "stars"); n != 5 { // STAR1 not stored without STAR1001
		t.Errorf("%d stars, want 5", n)
	}
}

func TestFindStarsChecksName(t *testing.T) {
	fetchStar = func(ctx context.Context, replicant, star string) (*model.Star, error) {
		return &model.Star{Designation: "OTHER"}, nil
	}
	t.Cleanup(func() { fetchStar = api.StarNear })
	if _, err := findStars(t.Context(), "NEAR", []string{"STAR1"}); err == nil {
		t.Error("wrong star accepted")
	}
}

func TestNearestReplicant(t *testing.T) {
	pos := map[string]model.Position{"A": {X: 5}, "B": {X: -2}, "C": {X: 3}}
	placed := []store.PlacedReplicant{{Replicant: "RA", Star: "A"}, {Replicant: "RB", Star: "B"}, {Replicant: "RC", Star: "C"}}
	if r, err := nearestReplicant(model.Position{}, placed, pos); err != nil || r.Replicant != "RB" {
		t.Errorf("got %+v, %v", r, err)
	}
	if _, err := nearestReplicant(model.Position{}, nil, pos); err == nil {
		t.Error("no replicants: accepted")
	}
}

// catchUp applies recorded events again, and only those of the type asked for.
func TestCatchUp(t *testing.T) {
	db := prospectStore(t)
	ctx := t.Context()
	r := prospectEvent("700-0", "STAR2")
	r.env.Payload = []byte(`{"origin":"INTERRANAE","stars":["STAR2"],"stars_generated":1}`)
	// Recorded before monitor could apply it.
	if _, _, err := store.RecordEvent(ctx, db, store.Event{ID: r.env.ID, Category: r.env.Category, Name: r.env.Event,
		CreatedAt: r.env.CreatedAt, Star: r.env.Star, Location: r.env.Location, DeviceCode: r.env.DeviceCode,
		DeviceType: r.env.DeviceType, Payload: r.env.Payload}); err != nil {
		t.Fatal(err)
	}
	fakeStars(t, 100)

	if err := catchUp(ctx, db, []string{"prospect.completed", "travel.cancelled"}, &printer{w: &strings.Builder{}}); err == nil {
		t.Error("catching up an event monitor doesn't handle: accepted")
	}
	var out strings.Builder
	if err := catchUp(ctx, db, []string{"prospect.completed"}, &printer{w: &out}); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM stars WHERE designation = 'STAR2'`).Scan(&n)
	if n != 1 {
		t.Error("STAR2 not stored")
	}
	if !strings.Contains(out.String(), "700-0 prospect.completed") {
		t.Errorf("output: %q", out.String())
	}
}
