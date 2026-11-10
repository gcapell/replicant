package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func TestDecideGaze(t *testing.T) {
	at := func(loc, status string) member {
		return member{Code: "OBS", Type: observatory, Location: loc, Status: status}
	}
	notYet := &store.Observation{Star: "T", Status: store.ObsNotYet}
	prospecting := &store.Observation{Star: "T", Status: store.ObsProspecting}
	state := func(o member, a *store.Observation) gazeState {
		s := gazeState{Obs: o, Active: a, HomeEntry: "H-1-L4"}
		if a != nil {
			s.TargetEntry = "T-2-L4"
		}
		return s
	}
	for _, c := range []struct {
		name string
		s    gazeState
		want gazeStep
	}{
		{"nothing under way, compacted: reserve", state(at("H-1-L4", "compacted"), nil), gazeReserve},
		{"nothing under way, unfurled: compact", state(at("H-1-L4", "idle"), nil), gazeCompact},
		{"nothing under way, compacting: wait", state(at("H-1-L4", "compacting"), nil), gazeWait},
		{"at home, compacted: move", state(at("H-1-L4", "compacted"), notYet), gazeMove},
		{"at home, unfurled: compact", state(at("H-1-L4", "idle"), notYet), gazeCompact},
		{"at home, unfurling: wait", state(at("H-1-L4", "unfurling"), notYet), gazeWait},
		{"at target, compacted: unfurl", state(at("T-2-L4", "compacted"), notYet), gazeUnfurl},
		{"at target, unfurled: prospect", state(at("T-2-L4", "idle"), notYet), gazeProspect},
		{"at target, prospecting (API says idle): wait", state(at("T-2-L4", "idle"), prospecting), gazeWait},
		{"at target, prospecting (store says so): wait", state(at("T-2-L4", "prospecting"), prospecting), gazeWait},
	} {
		a, err := decideGaze(c.s)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		} else if a.Step != c.want {
			t.Errorf("%s: step %d (%s), want %d", c.name, a.Step, a.Why, c.want)
		}
	}

	attached := at("T-2-L4", "compacted")
	attached.AttachedTo = "CARRIER"
	for name, s := range map[string]gazeState{
		"no location":                     state(at("", "compacted"), notYet),
		"attached":                        state(attached, notYet),
		"unknown status":                  state(at("T-2-L4", "warding"), notYet),
		"compacted away from entry point": state(at("H-3", "compacted"), notYet),
		"prospecting from elsewhere":      state(at("H-1-L4", "idle"), prospecting),
		"prospecting, no observation":     state(at("H-1-L4", "prospecting"), nil),
	} {
		if a, err := decideGaze(s); err == nil {
			t.Errorf("%s: accepted (%s)", name, a.Why)
		}
	}
}

func TestNextObservation(t *testing.T) {
	stars := []model.Star{
		{Designation: "A", Position: model.Position{X: 0}, EntryPoint: "A-1-L4"},
		{Designation: "B", Position: model.Position{X: 4}, EntryPoint: "B-1-L4"},
		{Designation: "C", Position: model.Position{X: 8}, EntryPoint: "C-1-L4"},
		{Designation: "NOENTRY", Position: model.Position{X: 100}},
		{Designation: "NORELAY", Position: model.Position{X: 4}, EntryPoint: "NORELAY-1-L4"},
	}
	relay := []string{"A", "B", "C", "NOENTRY", "OFFMAP"}
	// The centre is x=4 (A, B, C only): B is on it, then A and C tie, A by name.
	for _, c := range []struct {
		observed map[string]bool
		want     string
	}{
		{nil, "B -> B-1-L4 (0.0, 0.0, 0.0)"},
		{map[string]bool{"B": true}, "A -> A-1-L4 (-4.0, 0.0, 0.0)"},
		{map[string]bool{"B": true, "A": true}, "C -> C-1-L4 (4.0, 0.0, 0.0)"},
		{map[string]bool{"A": true, "B": true, "C": true}, "none"},
	} {
		got := "none"
		if p, ok := nextObservation(stars, relay, c.observed); ok {
			got = fmt.Sprintf("%s -> %s %s", p.Star, p.Entry, p.Dir)
		}
		if got != c.want {
			t.Errorf("observed %v: got %s, want %s", c.observed, got, c.want)
		}
	}
}

func TestRefusal(t *testing.T) {
	for _, c := range []struct {
		err    error
		status string
	}{
		{&api.HTTPError{StatusCode: 400, Body: []byte(`{"error":"too dense","ratio":0.9,"outward_ratio":0.8}`)}, store.ObsTooDense},
		{&api.HTTPError{StatusCode: 400, Body: []byte(`{"error":"Already surveyed from here"}`)}, store.ObsAlreadySurveyed},
		{&api.HTTPError{StatusCode: 400, Body: []byte(`{"error":"bad direction"}`)}, ""},
		{&api.HTTPError{StatusCode: 500, Body: []byte(`already`)}, ""},
		{fmt.Errorf("network"), ""},
	} {
		status, _, ok := refusal(c.err)
		if status != c.status || ok != (c.status != "") {
			t.Errorf("%v: %q %v, want %q", c.err, status, ok, c.status)
		}
	}
}

// gazeStore has relay stars HOME (x=0) and FAR (x=10), and an observatory
// OBS at HOME's entry point with a gaze task.
func gazeStore(t *testing.T) (*sql.DB, store.Task) {
	t.Helper()
	db := openStore(t)
	ctx := t.Context()
	for _, s := range []model.Star{
		{Designation: "HOME", Position: model.Position{X: 0}, EntryPoint: "HOME-1-L4"},
		{Designation: "FAR", Position: model.Position{X: 10}, EntryPoint: "FAR-2-L4"},
	} {
		if err := store.PutStar(ctx, db, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []model.Device{
		{DeviceCode: "RHOME", DeviceType: "ftl_relay", Status: "relaying", Location: "HOME-1-L4"},
		{DeviceCode: "RFAR", DeviceType: "ftl_relay", Status: "relaying", Location: "FAR-2-L4"},
		{DeviceCode: "OBS", DeviceType: observatory, Status: "idle", Location: "HOME-1-L4"},
	} {
		if err := store.PutDevice(ctx, db, d); err != nil {
			t.Fatal(err)
		}
	}
	// HOME was observed from long ago.
	exec(t, db, `INSERT INTO observations (star, status, created_at, updated_at) VALUES ('HOME', 'success', 0, 0)`)
	if err := fileGazeTasks(ctx, db, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	ts, err := store.TasksOfType(ctx, db, gazeTask)
	if err != nil || len(ts) != 1 {
		t.Fatalf("gaze tasks %v, %v", ts, err)
	}
	return db, ts[0]
}

// fakeObservatory answers /devices/OBS from *dev, and records the commands
// sent to it, with prospect answering with prospectErr.
func fakeObservatory(t *testing.T, dev *model.Device, prospectErr *error) *[]string {
	var sent []string
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		if code != dev.DeviceCode {
			return nil, fmt.Errorf("asked for %s", code)
		}
		d := *dev
		return &d, nil
	}
	compactCmd = func(ctx context.Context, code string) error { sent = append(sent, "compact "+code); return nil }
	unfurlCmd = func(ctx context.Context, code string) error { sent = append(sent, "unfurl "+code); return nil }
	prospectCmd = func(ctx context.Context, code string, dir model.Position) (*model.ProspectStarted, error) {
		sent = append(sent, fmt.Sprintf("prospect %s %s", code, dir))
		if *prospectErr != nil {
			return nil, *prospectErr
		}
		return &model.ProspectStarted{Status: "prospecting"}, nil
	}
	t.Cleanup(func() {
		fetchDevice, compactCmd, unfurlCmd, prospectCmd = api.Device, api.Compact, api.Unfurl, api.Prospect
	})
	return &sent
}

func observationRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT star, coalesce(device_code, ''), status, coalesce(dir_x, 0) FROM observations ORDER BY star`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var star, dev, status string
		var x float64
		if err := rows.Scan(&star, &dev, &status, &x); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s %s %s %.0f", star, dev, status, x))
	}
	return got
}

// A whole cycle: compact at HOME, reserve FAR, move there, unfurl, prospect.
func TestGazeCycle(t *testing.T) {
	db, task := gazeStore(t)
	ctx := t.Context()
	dev := &model.Device{DeviceCode: "OBS", DeviceType: observatory, Status: "idle", Location: "HOME-1-L4"}
	var prospectErr error
	sent := fakeObservatory(t, dev, &prospectErr)
	run := func() {
		t.Helper()
		if err := runTasks(ctx, db, []int64{task.ID}); err != nil {
			t.Fatal(err)
		}
	}

	run() // unfurled with nothing under way
	if !slices.Equal(*sent, []string{"compact OBS"}) {
		t.Fatalf("sent %v", *sent)
	}

	dev.Status = "compacted"
	run() // reserves FAR, runs again, files a move and blocks on it
	if got, want := observationRows(t, db), []string{"FAR OBS not_yet 5", "HOME  success 0"}; !slices.Equal(got, want) {
		t.Errorf("observations = %q, want %q", got, want)
	}
	if got, want := taskRows(t, db, "tasks"), []string{
		"1 gaze FAR-2-L4 blocked OBS",
		`2 move_devices HOME-1-L4 pending  {"dst":"FAR-2-L4","devices":["OBS"]}`,
	}; !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}

	// The move is done: the gaze task is unblocked at FAR.
	exec(t, db, `DELETE FROM tasks WHERE id = 2`)
	exec(t, db, `UPDATE tasks SET status = 'assigned' WHERE id = 1`)
	dev.Location = "FAR-2-L4"
	run()
	dev.Status = "idle"
	run()
	if want := []string{"compact OBS", "unfurl OBS", "prospect OBS (5.0, 0.0, 0.0)"}; !slices.Equal(*sent, want) {
		t.Errorf("sent %v, want %v", *sent, want)
	}
	if got, want := observationRows(t, db), []string{"FAR OBS prospecting 5", "HOME  success 0"}; !slices.Equal(got, want) {
		t.Errorf("observations = %q, want %q", got, want)
	}
	var status string
	db.QueryRow(`SELECT status FROM devices WHERE device_code = 'OBS'`).Scan(&status)
	if status != "prospecting" {
		t.Errorf("store has OBS %s, want prospecting", status)
	}

	run() // the API still says idle: wait
	if len(*sent) != 3 {
		t.Errorf("sent %v", *sent)
	}

	// prospect.completed: done, so compact, then nothing left to observe.
	exec(t, db, `UPDATE observations SET status = 'success' WHERE star = 'FAR'`)
	run()
	dev.Status = "compacted"
	run()
	if want := "compact OBS"; (*sent)[len(*sent)-1] != want || len(*sent) != 4 {
		t.Errorf("sent %v", *sent)
	}
}

func TestGazeRefused(t *testing.T) {
	db, task := gazeStore(t)
	ctx := t.Context()
	exec(t, db, `INSERT INTO observations (star, dir_x, dir_y, dir_z, device_code, status, created_at, updated_at)
		VALUES ('FAR', 5, 0, 0, 'OBS', 'not_yet', 0, 0)`)
	dev := &model.Device{DeviceCode: "OBS", DeviceType: observatory, Status: "idle", Location: "FAR-2-L4"}
	var prospectErr error = &api.HTTPError{StatusCode: 400, Body: []byte(`{"ratio":0.9,"outward_ratio":0.8}`)}
	sent := fakeObservatory(t, dev, &prospectErr)
	if err := runTasks(ctx, db, []int64{task.ID}); err != nil {
		t.Fatal(err)
	}
	// Refused, so it runs again and compacts.
	if want := []string{"prospect OBS (5.0, 0.0, 0.0)", "compact OBS"}; !slices.Equal(*sent, want) {
		t.Errorf("sent %v, want %v", *sent, want)
	}
	if got, want := observationRows(t, db), []string{"FAR OBS too_dense 5", "HOME  success 0"}; !slices.Equal(got, want) {
		t.Errorf("observations = %q, want %q", got, want)
	}

	// Any other error stops it.
	exec(t, db, `UPDATE observations SET status = 'not_yet' WHERE star = 'FAR'`)
	prospectErr = &api.HTTPError{StatusCode: 400, Body: []byte(`{"error":"bad direction"}`)}
	if err := runTasks(ctx, db, []int64{task.ID}); err == nil {
		t.Error("unknown refusal accepted")
	}
}

func TestFileGazeTasks(t *testing.T) {
	db, _ := gazeStore(t)
	ctx := t.Context()
	// Filing again adds nothing.
	if err := fileGazeTasks(ctx, db, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if got, want := taskRows(t, db, "tasks"), []string{"1 gaze HOME-1-L4 assigned OBS"}; !slices.Equal(got, want) {
		t.Errorf("tasks = %q, want %q", got, want)
	}
	// An observatory with some other task is an error.
	exec(t, db, `UPDATE tasks SET task = 'salvage' WHERE id = 1`)
	if err := fileGazeTasks(ctx, db, &strings.Builder{}); err == nil {
		t.Error("observatory with a salvage task: accepted")
	}
}

func TestNewPlantRelays(t *testing.T) {
	stars := []model.Star{
		{Designation: "R", Position: model.Position{X: 0}},
		{Designation: "A", Position: model.Position{X: 5}},
		{Designation: "B", Position: model.Position{X: 10}},
		{Designation: "FAR", Position: model.Position{X: 30}},
	}
	got := newPlantRelays(stars, []string{"R"}, map[string]bool{"A": true})
	if want := []string{"B"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v (A has a task, B joins via A, FAR is out of reach)", got, want)
	}
}
