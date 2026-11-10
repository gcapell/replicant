package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

// A survey's digests record its progress; the controller's
// directive.completed marks the system fully scanned, counting moons found
// beyond those we knew of, without asking the API about the system.
func TestSurveyCompletion(t *testing.T) {
	db := openStore(t)
	ctx := t.Context()
	const ctl = "DA921995"
	exec(t, db, `INSERT INTO stars (designation, x, y, z, entry_point, planet_count, planets_scanned, moon_count, moons_scanned)
		VALUES ('ULSIOS', 1, 2, 3, 'ULSIOS-7-L4', 7, 0, 58, 0)`)
	// Its survey task, and another team's at the same star.
	exec(t, db, `INSERT INTO tasks (id, task, location, status, phase, assignee, created_at, updated_at) VALUES
		(1, 'survey_system', 'ULSIOS', 'assigned', 'surveying', '`+ctl+`', 0, 5),
		(2, 'survey_system', 'ULSIOS', 'assigned', 'surveying', 'OTHER', 0, 5)`)
	phases := func() string {
		t.Helper()
		var p1, p2 string
		var u1 int64
		if err := db.QueryRow(`SELECT a.phase, a.updated_at, b.phase FROM tasks a, tasks b WHERE a.id = 1 AND b.id = 2`).Scan(&p1, &u1, &p2); err != nil {
			t.Fatal(err)
		}
		return p1 + " (" + strconv.FormatInt(u1, 10) + "), " + p2
	}
	if err := store.PutDevice(ctx, db, model.Device{DeviceCode: ctl, DeviceType: "ami_survey_controller", Status: "coordinating", Location: "ULSIOS-7-L4"}); err != nil {
		t.Fatal(err)
	}
	fetchDevice = func(ctx context.Context, code string) (*model.Device, error) {
		if code != ctl {
			return nil, errors.New("404 not found")
		}
		return &model.Device{DeviceCode: ctl, DeviceType: "ami_survey_controller", Status: "idle"}, nil
	}
	defer func() { fetchDevice = api.Device }()
	fetchLocation = func(ctx context.Context, star string) (*model.Location, error) {
		t.Errorf("read /locations/%s", star)
		return nil, errors.New("no")
	}
	defer func() { fetchLocation = api.Location }()

	n := 0
	send := func(name string, p any) error {
		n++
		e := ev(strings.Repeat("9", n)+"-0", name)
		e.DeviceCode, e.DeviceType, e.Star, e.Location = ctl, "ami_survey_controller", "ULSIOS", "ULSIOS-7-L4"
		return recordAll(ctx, db, feed(received{e, p}), &printer{w: &strings.Builder{}}, false)
	}
	counts := func() string {
		t.Helper()
		var full bool
		var ps, pc, ms, mc, ss, st int
		if err := db.QueryRow(`SELECT is_fully_scanned, planets_scanned, planet_count, moons_scanned, moon_count, survey_scanned, survey_total
			FROM stars WHERE designation = 'ULSIOS'`).Scan(&full, &ps, &pc, &ms, &mc, &ss, &st); err != nil {
			t.Fatal(err)
		}
		return strings.Join([]string{
			map[bool]string{true: "full", false: "partial"}[full],
			strconv.Itoa(ps) + "/" + strconv.Itoa(pc), strconv.Itoa(ms) + "/" + strconv.Itoa(mc), "survey " + strconv.Itoa(ss) + "/" + strconv.Itoa(st),
		}, " ")
	}
	digest := func(scanned, remaining, total int) *model.SurveyDigest {
		return &model.SurveyDigest{Report: model.SurveyReport{Progress: model.SurveyProgress{Scanned: scanned, Remaining: remaining, Total: total}}}
	}

	if err := send("ami.survey.digest", digest(30, 31, 61)); err != nil {
		t.Fatal(err)
	}
	if got, want := counts(), "partial 0/7 0/58 survey 30/61"; got != want {
		t.Errorf("part way: %s, want %s", got, want)
	}
	// Only this controller's task shows it, and updated_at stays put.
	if got, want := phases(), "surveying 30/61 (5), surveying"; got != want {
		t.Errorf("phases: %s, want %s", got, want)
	}
	// More turn up than we knew of (7 + 58 = 65).
	if err := send("ami.survey.digest", digest(60, 10, 70)); err != nil {
		t.Fatal(err)
	}
	// Another directive changes nothing.
	if err := send("directive.completed", &model.DirectiveCompleted{Directive: "patrol"}); err != nil {
		t.Fatal(err)
	}
	if got, want := counts(), "partial 0/7 0/58 survey 60/70"; got != want {
		t.Errorf("after patrol completed: %s, want %s", got, want)
	}
	// The extra 5 are taken to be moons.
	if err := send("directive.completed", &model.DirectiveCompleted{Directive: surveyDirective}); err != nil {
		t.Fatal(err)
	}
	if got, want := counts(), "full 7/7 63/63 survey 60/70"; got != want {
		t.Errorf("after survey_system completed: %s, want %s", got, want)
	}
}
