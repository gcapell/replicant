package main

// gaze tasks: a continual task holding a galactic_observatory, which it
// cycles through observation points (the task version of cmd/gaze). Each
// observation is a row of the observations table. The observatory prospects
// from a relay star's entry point, pointing straight out from the centre of
// the relay stars; it's carried between entry points by a move_devices task,
// and never moves within a system.
//
// Every sweep files a gaze task, assigned to it, for each observatory with
// no task (fileGazeTasks).

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

const (
	gazeTask    = "gaze"
	observatory = "galactic_observatory"
)

// Observatory statuses. An unfurled observatory reports idle, even while
// prospecting, so whether it's prospecting comes from its observation.
const (
	obsCompacted  = "compacted"
	obsCompacting = "compacting"
	obsUnfurling  = "unfurling"
	obsIdle       = "idle"
	obsProspect   = "prospecting" // what we set in the store; the API may say idle
)

// Commands, replaceable in tests.
var (
	compactCmd  = api.Compact
	unfurlCmd   = api.Unfurl
	prospectCmd = api.Prospect
)

// gaze is the gaze task's handler. It fetches the observatory, reads its
// observation under way (if any), and takes the step decideGaze picks.
func gaze(ctx context.Context, db store.Reader, t store.Task) (taskChange, error) {
	o, err := fetchMember(ctx, t.Assignee, observatory, "")
	if err != nil {
		return nil, err
	}
	s := gazeState{Obs: o}
	obs, ok, err := store.ActiveObservation(ctx, db, o.Code)
	if err != nil {
		return nil, err
	}
	if ok {
		s.Active = &obs
		if s.TargetEntry, err = store.EntryPoint(ctx, db, obs.Star); err != nil {
			return nil, err
		}
	}
	if o.Location != "" {
		if s.HomeEntry, err = store.EntryPoint(ctx, db, auto.StarOf(o.Location)); err != nil {
			return nil, err
		}
	}
	a, err := decideGaze(s)
	if err != nil {
		return nil, err
	}
	log.Printf("task %d %s %s (%s): %s", t.ID, t.Task, t.Location, t.Assignee, a.Why)

	switch a.Step {
	case gazeWait:
		return nil, nil
	case gazeCompact:
		if err := compactCmd(ctx, o.Code); err != nil {
			return nil, err
		}
		return setPhase(t, "compacting"), nil
	case gazeUnfurl:
		if err := unfurlCmd(ctx, o.Code); err != nil {
			return nil, err
		}
		return setPhase(t, "unfurling"), nil
	case gazeProspect:
		return startProspect(ctx, t, *s.Active)
	case gazeReserve:
		return reserveNext(ctx, db, t)
	case gazeMove:
		return blockOnMove(t, s.HomeEntry, s.TargetEntry, []string{o.Code}), nil
	}
	return nil, fmt.Errorf("unknown step %d", a.Step)
}

type gazeStep int

const (
	gazeWait     gazeStep = iota // nothing to do until something happens
	gazeCompact                  // send compact
	gazeUnfurl                   // send unfurl
	gazeProspect                 // send prospect, in the observation's direction
	gazeReserve                  // choose and reserve the next star to observe from
	gazeMove                     // file a move_devices task to the target's entry point and wait for it
)

type gazeAction struct {
	Step gazeStep
	Why  string // for the log
}

type gazeState struct {
	Obs         member
	Active      *store.Observation // its observation under way; nil if none
	TargetEntry string             // Active's star's entry point
	HomeEntry   string             // the entry point of the star Obs is at
}

// decideGaze is the gaze task's next step:
//
//	no observation under way:
//	  compacted? reserve the next star
//	  unfurled (idle)? compact
//	at the target's entry point:
//	  compacted? unfurl
//	  unfurled and not yet prospecting? prospect
//	elsewhere:
//	  unfurled? compact
//	  compacted at its own system's entry point? move to the target's
//	compacting or unfurling, or prospecting: wait
//
// Anything else is an error: an observatory with no location, attached to
// a carrier, compacted away from an entry point, an unknown status, or an
// observation prospecting from somewhere the observatory isn't.
func decideGaze(s gazeState) (gazeAction, error) {
	o := s.Obs
	if o.Location == "" {
		return gazeAction{}, fmt.Errorf("observatory %s is %s with no location", o.Code, o.Status)
	}
	if o.AttachedTo != "" {
		return gazeAction{}, fmt.Errorf("observatory %s is attached to %s at %s", o.Code, o.AttachedTo, o.Location)
	}
	switch o.Status {
	case obsCompacting, obsUnfurling:
		return gazeAction{Step: gazeWait, Why: "waiting: " + o.Status}, nil
	case obsCompacted, obsIdle, obsProspect:
	default:
		return gazeAction{}, fmt.Errorf("observatory %s has status %q", o.Code, o.Status)
	}
	a := s.Active
	if a != nil && a.Status == store.ObsProspecting {
		if o.Location != s.TargetEntry {
			return gazeAction{}, fmt.Errorf("observatory %s is at %s, but prospecting from %s (%s)", o.Code, o.Location, a.Star, s.TargetEntry)
		}
		return gazeAction{Step: gazeWait, Why: "waiting: prospecting from " + a.Star}, nil
	}
	if o.Status == obsProspect {
		return gazeAction{}, fmt.Errorf("observatory %s is prospecting at %s, but has no observation prospecting", o.Code, o.Location)
	}
	unfurled := o.Status == obsIdle

	switch {
	case a == nil && unfurled:
		return gazeAction{Step: gazeCompact, Why: "no observation under way; compacting"}, nil
	case a == nil:
		return gazeAction{Step: gazeReserve, Why: "compacted; choosing the next star"}, nil
	case o.Location == s.TargetEntry && unfurled:
		return gazeAction{Step: gazeProspect, Why: fmt.Sprintf("prospecting from %s direction %s", a.Star, a.Dir)}, nil
	case o.Location == s.TargetEntry:
		return gazeAction{Step: gazeUnfurl, Why: "at " + s.TargetEntry + "; unfurling"}, nil
	case unfurled:
		return gazeAction{Step: gazeCompact, Why: fmt.Sprintf("at %s, not %s; compacting", o.Location, s.TargetEntry)}, nil
	case o.Location != s.HomeEntry:
		return gazeAction{}, fmt.Errorf("observatory %s is compacted at %s, not at its system's entry point %s", o.Code, o.Location, s.HomeEntry)
	}
	return gazeAction{Step: gazeMove, Why: fmt.Sprintf("at %s; filing %s to %s", o.Location, moveDevicesTask, s.TargetEntry)}, nil
}

// observationPoint is a star to observe from.
type observationPoint struct {
	Star  string
	Entry string
	Dir   model.Position
}

// nextObservation is the relay star nearest the centre of the relay stars
// (ties by name) that has no observation, with the direction straight out
// from the centre through it. Relay stars missing from stars, or with no
// entry point, are left out (and from the centre too). ok is false if there's
// none left.
func nextObservation(stars []model.Star, relay []string, observed map[string]bool) (p observationPoint, ok bool) {
	var cands []model.Star
	for _, s := range stars {
		if slices.Contains(relay, s.Designation) && s.EntryPoint != "" {
			cands = append(cands, s)
		}
	}
	if len(cands) == 0 {
		return p, false
	}
	var c model.Position
	for _, s := range cands {
		c.X += s.Position.X
		c.Y += s.Position.Y
		c.Z += s.Position.Z
	}
	n := float64(len(cands))
	c = model.Position{X: c.X / n, Y: c.Y / n, Z: c.Z / n}
	cands = slices.DeleteFunc(cands, func(s model.Star) bool { return observed[s.Designation] })
	if len(cands) == 0 {
		return p, false
	}
	best := slices.MinFunc(cands, func(a, b model.Star) int {
		return cmp.Or(cmp.Compare(a.Position.Distance(c), b.Position.Distance(c)), cmp.Compare(a.Designation, b.Designation))
	})
	return observationPoint{Star: best.Designation, Entry: best.EntryPoint, Dir: best.Position.Delta(c)}, true
}

// reserveNext chooses the next star for t's observatory, and returns a change
// reserving it, moving t there and running t again. With no star left it
// logs that and waits.
func reserveNext(ctx context.Context, db store.Reader, t store.Task) (taskChange, error) {
	stars, err := store.Stars(ctx, db)
	if err != nil {
		return nil, err
	}
	relay, err := store.RelayStars(ctx, db)
	if err != nil {
		return nil, err
	}
	observed, err := store.ObservedStars(ctx, db)
	if err != nil {
		return nil, err
	}
	p, ok := nextObservation(stars, relay, observed)
	if !ok {
		log.Printf("task %d %s (%s): no relay star left to observe from", t.ID, t.Task, t.Assignee)
		return nil, nil
	}
	return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
		now := time.Now()
		if err := store.ReserveObservation(ctx, tx, p.Star, p.Dir, t.Assignee, now); err != nil {
			return nil, err
		}
		if err := store.SetTaskLocation(ctx, tx, t.ID, p.Entry, now); err != nil {
			return nil, err
		}
		if err := store.SetPhase(ctx, tx, t.ID, "reserved", now); err != nil {
			return nil, err
		}
		log.Printf("task %d %s: %s will observe from %s (%s) direction %s", t.ID, t.Task, t.Assignee, p.Star, p.Entry, p.Dir)
		return []int64{t.ID}, nil
	}, nil
}

// startProspect sends prospect for observation a. If it's accepted, the
// observation and the observatory become prospecting (we get no event for
// that). If it's refused for good (too dense, or already surveyed from), the
// observation records why, and the task runs again (to compact). Any other
// error is the handler's.
func startProspect(ctx context.Context, t store.Task, a store.Observation) (taskChange, error) {
	r, err := prospectCmd(ctx, t.Assignee, a.Dir)
	status := store.ObsProspecting
	if err == nil {
		log.Printf("%s prospecting from %s: %s, completes %s", t.Assignee, a.Star, r.Status, r.CompletesAt)
	} else {
		var note string
		var ok bool
		if status, note, ok = refusal(err); !ok {
			return nil, err
		}
		log.Printf("%s refused to prospect from %s: %s", t.Assignee, a.Star, note)
	}
	return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
		now := time.Now()
		if err := store.SetObservationStatus(ctx, tx, a.Star, t.Assignee, store.ObsNotYet, status, now); err != nil {
			return nil, err
		}
		if status != store.ObsProspecting {
			return []int64{t.ID}, store.SetPhase(ctx, tx, t.ID, status, now)
		}
		if err := store.SetStatus(ctx, tx, t.Assignee, obsProspect); err != nil {
			return nil, err
		}
		return nil, store.SetPhase(ctx, tx, t.ID, "prospecting", now)
	}, nil
}

var ratioRE = regexp.MustCompile(`"((?:outward_)?ratio)"\s*:\s*([-0-9.eE+]+)`)

// refusal reports whether err is prospect refusing to run here for good: the
// area is too dense (a 400 reporting ratios), or this star has already been
// surveyed from. It returns the observation's status, and a note for the log.
func refusal(err error) (status, note string, ok bool) {
	var he *api.HTTPError
	if !errors.As(err, &he) || he.StatusCode != 400 {
		return "", "", false
	}
	body := string(he.Body)
	if m := ratioRE.FindAllStringSubmatch(body, -1); m != nil {
		note := "too dense:"
		for _, kv := range m {
			note += " " + kv[1] + "=" + kv[2]
		}
		return store.ObsTooDense, note, true
	}
	lower := strings.ToLower(body)
	if strings.Contains(lower, "already") || strings.Contains(lower, "surveyed") {
		return store.ObsAlreadySurveyed, "already surveyed from: " + body, true
	}
	return "", "", false
}

// fileGazeTasks files a gaze task, assigned to it, for each
// galactic_observatory in the store with no task, at its observation's entry
// point if it has one under way, else where it is. One with a task of another
// type is an error.
func fileGazeTasks(ctx context.Context, db *sql.DB, w io.Writer) error {
	return inTx(ctx, db, func(tx *sql.Tx) error {
		obs, err := store.DevicesOfType(ctx, tx, observatory)
		if err != nil {
			return err
		}
		now := time.Now()
		for _, d := range obs {
			t, ok, err := store.TaskOfAssignee(ctx, tx, d.Code)
			if err != nil {
				return err
			}
			if ok {
				if t.Task != gazeTask {
					return fmt.Errorf("%s %s is assigned task %d (%s at %s)", observatory, d.Code, t.ID, t.Task, t.Location)
				}
				continue
			}
			loc := d.Location
			if a, ok, err := store.ActiveObservation(ctx, tx, d.Code); err != nil {
				return err
			} else if ok {
				if loc, err = store.EntryPoint(ctx, tx, a.Star); err != nil {
					return err
				}
			}
			id, err := store.AddTask(ctx, tx, gazeTask, loc, "", store.DefaultPriority, now)
			if err != nil {
				return err
			}
			if err := store.AssignTask(ctx, tx, id, d.Code, now); err != nil {
				return err
			}
			fmt.Fprintf(w, "  new %s %d %s <- %s\n", gazeTask, id, loc, d.Code)
		}
		return nil
	})
}
