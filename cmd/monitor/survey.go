package main

// Startup's first step towards the task executor (design/tasks.md): file a
// survey_system task for every relay star that isn't fully surveyed, and give
// the free survey teams one each. Nothing carries the tasks out yet.

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

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
			if ids[star], err = store.AddTask(ctx, tx, surveyTask, star, now); err != nil {
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
