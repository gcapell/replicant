package main

// survey_system tasks (design/tasks.md). At startup, fileSurveys files one
// for every relay star that isn't fully surveyed, and gives the free survey
// teams one each. surveySystem is the task's handler.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

const surveyTask = "survey_system"

type surveyAssignment struct {
	Star     string
	Team     store.SurveyTeam
	Distance float64 // ly, from the team's star
}

type surveyPlan struct {
	New      []string              // stars getting a new survey_system task
	Pending  map[string]store.Task // existing pending, unassigned tasks, by star
	Assign   []surveyAssignment
	Waiting  []string // stars whose task stays pending (no team for it)
	Existing int      // survey_system tasks already filed, of any status
}

// planSurveys works out which survey_system tasks to add (one per unsurveyed
// star that has no task, even a failed one, which waits for a human), and
// pairs free teams with the pending tasks, new and old, minimising the
// longest trip and then the total. Each team gets at most one task.
func planSurveys(unsurveyed []string, existing []store.Task, teams []store.SurveyTeam, pos map[string]model.Position) (surveyPlan, error) {
	p := surveyPlan{Pending: map[string]store.Task{}, Existing: len(existing)}
	hasTask := map[string]bool{}
	for _, t := range existing {
		hasTask[t.Location] = true
		if t.Status == "pending" && t.Assignee == "" {
			p.Pending[t.Location] = t
		}
	}
	for _, star := range unsurveyed {
		if !hasTask[star] {
			p.New = append(p.New, star)
		}
	}
	open := slices.Concat(p.New, slices.Sorted(maps.Keys(p.Pending)))
	for _, s := range open {
		if _, ok := pos[s]; !ok {
			return p, fmt.Errorf("no position for star %s", s)
		}
	}
	for _, t := range teams {
		if _, ok := pos[t.Star]; !ok {
			return p, fmt.Errorf("no position for star %s (team %s)", t.Star, t.Controller)
		}
	}
	dist := func(a, t int) float64 { return pos[teams[a].Star].Distance(pos[open[t]]) }
	taken := map[string]bool{}
	for a, t := range auto.MatchMinMax(len(teams), len(open), dist) {
		if t >= 0 {
			p.Assign = append(p.Assign, surveyAssignment{Star: open[t], Team: teams[a], Distance: dist(a, t)})
			taken[open[t]] = true
		}
	}
	slices.SortFunc(p.Assign, func(a, b surveyAssignment) int { return strings.Compare(a.Star, b.Star) })
	for _, s := range open {
		if !taken[s] {
			p.Waiting = append(p.Waiting, s)
		}
	}
	slices.Sort(p.Waiting)
	return p, nil
}

// readSurveyPlan gathers what planSurveys needs from the store and plans.
func readSurveyPlan(ctx context.Context, q store.Reader) (surveyPlan, error) {
	unsurveyed, err := store.UnsurveyedRelayStars(ctx, q)
	if err != nil {
		return surveyPlan{}, err
	}
	existing, err := store.TasksOfType(ctx, q, surveyTask)
	if err != nil {
		return surveyPlan{}, err
	}
	teams, err := store.FreeSurveyTeams(ctx, q)
	if err != nil {
		return surveyPlan{}, err
	}
	pos := map[string]model.Position{}
	stars := slices.Clone(unsurveyed)
	for _, t := range existing {
		stars = append(stars, t.Location)
	}
	for _, t := range teams {
		stars = append(stars, t.Star)
	}
	for _, s := range stars {
		if _, ok := pos[s]; ok {
			continue
		}
		if pos[s], err = store.StarPosition(ctx, q, s); err != nil {
			return surveyPlan{}, err
		}
	}
	return planSurveys(unsurveyed, existing, teams, pos)
}

// fileSurveys plans and stores survey_system tasks in one transaction.
func fileSurveys(ctx context.Context, db *sql.DB, w io.Writer) error {
	return inTx(ctx, db, func(tx *sql.Tx) error {
		p, err := readSurveyPlan(ctx, tx)
		if err != nil {
			return err
		}
		now := time.Now()
		ids := map[string]int64{}
		for star, t := range p.Pending {
			ids[star] = t.ID
		}
		for _, star := range p.New {
			if ids[star], err = store.AddTask(ctx, tx, surveyTask, star, "", now); err != nil {
				return err
			}
		}
		for _, a := range p.Assign {
			if err := store.AssignTask(ctx, tx, ids[a.Star], a.Team.Controller, now); err != nil {
				return err
			}
		}
		p.print(w)
		return nil
	})
}

func (p surveyPlan) print(w io.Writer) {
	fmt.Fprintf(w, "survey tasks: %d already filed, %d new, %d to assign, %d left pending\n",
		p.Existing, len(p.New), len(p.Assign), len(p.Waiting))
	for _, s := range p.New {
		fmt.Fprintf(w, "  new %s %s\n", surveyTask, s)
	}
	for _, a := range p.Assign {
		fmt.Fprintf(w, "  %s %s <- %s (%d survey_drones at %s, %.1f ly)\n",
			surveyTask, a.Star, a.Team.Controller, a.Team.Drones, a.Team.Location, a.Distance)
	}
	for _, s := range p.Waiting {
		fmt.Fprintf(w, "  %s %s: pending, no free team\n", surveyTask, s)
	}
}

// surveyDirective is the AMI directive that scans every planet and moon in
// the controller's system.
const surveyDirective = "survey_system"

// surveySystem is the survey_system task's handler: t.Assignee is an
// ami_survey_controller, which with its survey_drones scans every planet and
// moon at t.Location (a star). Each call takes one step (decideSurvey); the
// team's devices are fetched from the API first, so it acts on how they are
// now, not on the store.
func surveySystem(ctx context.Context, db store.Reader, t store.Task) (change, error) {
	drones, err := store.ControlledBy(ctx, db, t.Assignee)
	if err != nil {
		return nil, err
	}
	team := make([]member, 0, 1+len(drones))
	c, err := fetchMember(ctx, t.Assignee, "ami_survey_controller", "")
	if err != nil {
		return nil, err
	}
	team = append(team, c)
	for _, d := range drones {
		m, err := fetchMember(ctx, d, "survey_drone", t.Assignee)
		if err != nil {
			return nil, err
		}
		team = append(team, m)
	}
	s := surveyState{Target: t.Location, Team: team}
	if s.TargetEntry, err = store.EntryPoint(ctx, db, t.Location); err != nil {
		return nil, err
	}
	if s.Complete, err = store.IsFullyScanned(ctx, db, t.Location); err != nil {
		return nil, err
	}
	if c.Location != "" && auto.StarOf(c.Location) != t.Location {
		if s.HomeEntry, err = store.EntryPoint(ctx, db, auto.StarOf(c.Location)); err != nil {
			return nil, err
		}
	}
	a, err := decideSurvey(s)
	if err != nil {
		return nil, err
	}
	log.Printf("task %d %s %s (%s): %s", t.ID, t.Task, t.Location, t.Assignee, a.Why)

	switch a.Step {
	case surveyWait:
		if c.Directive == surveyDirective {
			return setPhase(t, phaseSurveying), nil
		}
		return nil, nil
	case surveyStart:
		if err := api.SurveySystem(ctx, t.Assignee); err != nil {
			return nil, err
		}
		return setPhase(t, phaseSurveying), nil
	case surveyGather:
		for _, d := range a.Travellers {
			if err := api.Travel(ctx, d, a.Dest); err != nil {
				return nil, err
			}
		}
		if a.Dest == s.TargetEntry {
			return setPhase(t, phaseRegrouping), nil
		}
		return setPhase(t, phaseGathering), nil
	case surveyDone:
		return func(ctx context.Context, tx *sql.Tx) error {
			if _, err := store.FinishTask(ctx, tx, t.ID, time.Now()); err != nil {
				return err
			}
			_, err := assignNext(ctx, tx, t.Assignee, "ami_survey_controller", t.Location, time.Now())
			return err
		}, nil
	case surveyBlock:
		codes := make([]string, len(team))
		for i, m := range team {
			codes[i] = m.Code
		}
		return blockOnMove(t.ID, a.Dest, s.TargetEntry, codes), nil
	}
	return nil, fmt.Errorf("unknown step %d", a.Step)
}

// survey_system phases: gathering at the entry point of the system the
// team is in (to be picked up), surveying, then regrouping at the target's
// entry point. While a move brings the team over, the task is blocked.
const (
	phaseGathering  = "gathering"
	phaseSurveying  = "surveying"
	phaseRegrouping = "regrouping"
)

type surveyStep int

const (
	surveyWait   surveyStep = iota // nothing to do until something happens
	surveyStart                    // send survey_system to the controller
	surveyGather                   // send Travellers to Dest
	surveyDone                     // finish the task, find the team another
	surveyBlock                    // wait at Dest for a move_devices task to bring the team over
)

type surveyAction struct {
	Step       surveyStep
	Travellers []string // surveyGather: who to send
	Dest       string   // surveyGather: where to; surveyBlock: where the team waits
	Why        string   // for the log
}

type surveyState struct {
	Target      string   // the star to survey
	TargetEntry string   // its entry point
	HomeEntry   string   // the entry point of the controller's star, if that isn't Target
	Complete    bool     // every planet and moon at Target is scanned
	Team        []member // the controller first, then its drones
}

// decideSurvey is the survey_system task's next step:
//
//	in the target system?
//	  directive running? wait
//	  survey incomplete? start the survey
//	  anyone away from the entry point? send them there
//	  done
//	elsewhere: anyone away from our own entry point? send them there
//	  else file a move_devices task and wait for it
//	a member with no location (travelling or carried)? wait until it arrives
//
// Anything unexpected is an error: a team split across systems, a member
// attached to a carrier when it needs to act, or the controller running a
// directive other than survey_system (or any directive outside the target).
func decideSurvey(s surveyState) (surveyAction, error) {
	if len(s.Team) == 0 {
		return surveyAction{}, errors.New("no team")
	}
	c := s.Team[0]
	for _, m := range s.Team {
		if m.Location == "" {
			return surveyAction{Step: surveyWait, Why: fmt.Sprintf("waiting: %s is %s with no location (destination %q)", m.Code, m.Status, m.Destination)}, nil
		}
	}
	star := auto.StarOf(c.Location)
	for _, m := range s.Team {
		if auto.StarOf(m.Location) != star {
			return surveyAction{}, fmt.Errorf("team split: controller %s is at %s, but %s is at %s", c.Code, c.Location, m.Code, m.Location)
		}
	}
	if c.Directive != "" && c.Directive != surveyDirective {
		return surveyAction{}, fmt.Errorf("controller %s is running directive %s", c.Code, c.Directive)
	}

	if star == s.Target {
		for _, m := range s.Team {
			if m.AttachedTo != "" {
				return surveyAction{}, fmt.Errorf("%s is still attached to %s in the target system", m.Code, m.AttachedTo)
			}
		}
		if c.Directive == surveyDirective {
			return surveyAction{Step: surveyWait, Why: "waiting: survey under way"}, nil
		}
		if !s.Complete {
			return surveyAction{Step: surveyStart, Why: "starting " + surveyDirective}, nil
		}
		if away := awayFrom(s.Team, s.TargetEntry); len(away) > 0 {
			return gather(away, s.TargetEntry, "survey complete; gathering at"), nil
		}
		return surveyAction{Step: surveyDone, Why: "survey complete, team at " + s.TargetEntry}, nil
	}

	if c.Directive != "" {
		return surveyAction{}, fmt.Errorf("controller %s is running %s at %s, not %s", c.Code, c.Directive, star, s.Target)
	}
	if s.HomeEntry == "" {
		return surveyAction{}, fmt.Errorf("no entry point for %s", star)
	}
	away := awayFrom(s.Team, s.HomeEntry)
	if len(away) == 0 {
		return surveyAction{Step: surveyBlock, Dest: s.HomeEntry,
			Why: fmt.Sprintf("team at %s; filing move_devices to %s", s.HomeEntry, s.TargetEntry)}, nil
	}
	for _, m := range s.Team {
		if m.AttachedTo != "" && slices.Contains(away, m.Code) {
			return surveyAction{}, fmt.Errorf("%s is attached to %s at %s, away from %s", m.Code, m.AttachedTo, m.Location, s.HomeEntry)
		}
	}
	return gather(away, s.HomeEntry, "not in "+s.Target+"; gathering at"), nil
}

func awayFrom(team []member, loc string) []string {
	var away []string
	for _, m := range team {
		if m.Location != loc {
			away = append(away, m.Code)
		}
	}
	return away
}

func gather(away []string, dest, why string) surveyAction {
	return surveyAction{Step: surveyGather, Travellers: away, Dest: dest,
		Why: fmt.Sprintf("%s %s: sending %s", why, dest, strings.Join(away, " "))}
}
