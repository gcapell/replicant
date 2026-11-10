package main

// salvage tasks (design/tasks.md): an ami_mining_controller, its
// mining_drones and its maintenance_drone (adopted_maintenance_drones)
// collect the salvage at one location, a planet or moon. There's one task per
// location. A salvage.discovered event files one (salvageDiscovered), as does
// startup with -startup_sites (findSalvageSites, from /locations).
// assignSalvage gives pending ones to free teams; salvage is the handler.

import (
	"context"
	"database/sql"
	"encoding/json"
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

const (
	salvageTask      = "salvage"
	salvageDirective = "gather_salvage"
	patrolDirective  = "patrol"

	// salvagePriority is the priority of salvage tasks, and of the moves
	// they file.
	salvagePriority = 4

	// minSalvageMiners is how many mining_drones a salvage team needs.
	minSalvageMiners = 4
)

// salvageDetails is a salvage task's details: what each salvage site there
// held when salvage.discovered reported it, by designation, e.g.
// {"OTHYONAL-3-1-SAL-1": "Co=127 R=93 S=150"}. A task filed from /locations
// has no details.
type salvageDetails struct {
	Sites map[string]string `json:"sites"`
}

// --- filing ---

// salvageDiscovered files a salvage task at the payload's location (a planet
// or moon of the envelope's star), or adds the site to the one already there.
// The envelope's device is the survey controller that found it, and its
// location is where that controller is.
func salvageDiscovered(ctx context.Context, tx *sql.Tx, env api.Envelope, payload any) error {
	p, ok := payload.(*model.SalvageDiscovered)
	if !ok {
		return fmt.Errorf("payload is %T", payload)
	}
	if env.Star == "" || !auto.IsPlanetOrMoon(env.Star, p.Location) {
		return fmt.Errorf("salvage at %q, which isn't a planet or moon of star %q", p.Location, env.Star)
	}
	if !strings.HasPrefix(p.Designation, p.Location+"-") {
		return fmt.Errorf("salvage %q isn't at its location %s", p.Designation, p.Location)
	}
	res, err := formatResources(p.Resources)
	if err != nil {
		return fmt.Errorf("salvage %s: %w", p.Designation, err)
	}
	return fileSalvage(ctx, tx, p.Location, p.Designation, res, time.Now())
}

// formatResources shows resources as C=54 S=109 V=43 (README's order). An
// unknown resource type is an error.
func formatResources(rs map[model.ResourceType]int) (string, error) {
	for r := range rs {
		if !slices.Contains(model.Resources, r) {
			return "", fmt.Errorf("unknown resource type %q", r)
		}
	}
	var parts []string
	for _, r := range model.Resources {
		if n := rs[r]; n != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", r.Abbrev(), n))
		}
	}
	return strings.Join(parts, " "), nil
}

// fileSalvage files a pending salvage task at loc, unless there's one there
// already (of any status). If designation isn't "", the site and its
// resources go in the task's details, new or existing.
func fileSalvage(ctx context.Context, tx *sql.Tx, loc, designation, resources string, now time.Time) error {
	t, exists, err := store.TaskAt(ctx, tx, salvageTask, loc)
	if err != nil {
		return err
	}
	var d salvageDetails
	if exists && t.Details != "" {
		if err := json.Unmarshal([]byte(t.Details), &d); err != nil {
			return fmt.Errorf("task %d: details %q: %w", t.ID, t.Details, err)
		}
	}
	if designation != "" {
		if d.Sites == nil {
			d.Sites = map[string]string{}
		}
		if exists && d.Sites[designation] == resources {
			return nil // a repeat (e.g. -catchup)
		}
		d.Sites[designation] = resources
	} else if exists {
		return nil
	}
	details := ""
	if len(d.Sites) > 0 {
		b, err := json.Marshal(d)
		if err != nil {
			return err
		}
		details = string(b)
	}
	if exists {
		log.Printf("task %d %s %s: adding %s (%s)", t.ID, salvageTask, loc, designation, resources)
		return store.SetDetails(ctx, tx, t.ID, details, now)
	}
	id, err := store.AddTask(ctx, tx, salvageTask, loc, details, salvagePriority, now)
	if err != nil {
		return err
	}
	log.Printf("new task %d %s %s %s", id, salvageTask, loc, details)
	return nil
}

// listLocations is api.Locations, replaceable in tests.
var listLocations = api.Locations

// findSalvageSites files a salvage task (with no details) for each location
// /locations says has resource sites (chooseSalvageSites) that has no salvage
// task yet. Sites at stars without a relay are listed, not filed.
func findSalvageSites(ctx context.Context, db *sql.DB, w io.Writer) error {
	list, err := listLocations(ctx)
	if err != nil {
		return err
	}
	return inTx(ctx, db, func(tx *sql.Tx) error {
		relays, err := store.RelayStars(ctx, tx)
		if err != nil {
			return err
		}
		sites, unrelayed := chooseSalvageSites(list.Locations, relays)
		filed := 0
		now := time.Now()
		for _, loc := range sites {
			_, exists, err := store.TaskAt(ctx, tx, salvageTask, loc)
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			if err := fileSalvage(ctx, tx, loc, "", "", now); err != nil {
				return err
			}
			filed++
		}
		fmt.Fprintf(w, "salvage sites: %d at relay stars (%d new tasks)\n", len(sites), filed)
		if len(unrelayed) > 0 {
			fmt.Fprintf(w, "  not filed, no relay: %s\n", strings.Join(unrelayed, " "))
		}
		return nil
	})
}

// chooseSalvageSites picks the planets and moons with resource sites (as
// auto_salvage did, never at SOL), sorted, split into those at relay stars
// and the rest.
func chooseSalvageSites(locs map[string]model.LocationSummary, relays []string) (sites, unrelayed []string) {
	for loc, s := range locs {
		star := auto.StarOf(loc)
		if s.ResourceSites == 0 || star == "SOL" || !auto.IsPlanetOrMoon(star, loc) {
			continue
		}
		if slices.Contains(relays, star) {
			sites = append(sites, loc)
		} else {
			unrelayed = append(unrelayed, loc)
		}
	}
	slices.Sort(sites)
	slices.Sort(unrelayed)
	return sites, unrelayed
}

// --- assigning ---

// salvageTeamProblem says why an idle mining team can't salvage, or "" if it
// can. A controller that controls anything but mining_drones (and its own
// maintenance_drone) is an error. maxCapacity is the largest transport's.
func salvageTeamProblem(t store.MiningTeam, maxCapacity int) (string, error) {
	if len(t.Others) > 0 {
		return "", fmt.Errorf("ami_mining_controller %s@%s controls %s, not just mining_drones", t.Controller, t.Location, strings.Join(t.Others, ", "))
	}
	switch {
	case t.Location == "":
		return "has no location (travelling or stowed)", nil
	case t.Miners < minSalvageMiners:
		return fmt.Sprintf("has %d mining_drones, under %d", t.Miners, minSalvageMiners), nil
	case t.Maint == "":
		return "has no maintenance_drone in adopted_maintenance_drones", nil
	case t.MaintStar != t.Star:
		loc := t.MaintLocation
		if loc == "" {
			loc = "no location"
		}
		return fmt.Sprintf("its maintenance_drone %s@%s isn't in %s", t.Maint, loc, t.Star), nil
	case t.Miners+2 > maxCapacity:
		return fmt.Sprintf("is %d devices (controller, maintenance_drone, %d mining_drones), more than our largest transport carries (%d)",
			t.Miners+2, t.Miners, maxCapacity), nil
	}
	return "", nil
}

type salvageAssignment struct {
	Task     store.Task
	Team     store.MiningTeam
	Distance float64 // ly, from the team's star
}

type salvagePlan struct {
	Assign  []salvageAssignment
	Waiting []store.Task      // pending tasks left without a team
	Invalid map[string]string // idle teams that can't salvage, by controller@location: why
	Teams   int               // valid free teams
}

// planSalvage pairs free teams with pending salvage tasks, at most one each,
// as many as it can, then minimising the longest trip and then the total
// (star to star).
func planSalvage(tasks []store.Task, teams []store.MiningTeam, pos map[string]model.Position) (salvagePlan, error) {
	p := salvagePlan{Teams: len(teams)}
	for _, t := range tasks {
		if _, ok := pos[auto.StarOf(t.Location)]; !ok {
			return p, fmt.Errorf("no position for star %s (task %d)", auto.StarOf(t.Location), t.ID)
		}
	}
	for _, t := range teams {
		if _, ok := pos[t.Star]; !ok {
			return p, fmt.Errorf("no position for star %s (team %s)", t.Star, t.Controller)
		}
	}
	dist := func(a, i int) float64 { return pos[teams[a].Star].Distance(pos[auto.StarOf(tasks[i].Location)]) }
	taken := make([]bool, len(tasks))
	for a, i := range auto.MatchMinMax(len(teams), len(tasks), dist) {
		if i >= 0 {
			p.Assign = append(p.Assign, salvageAssignment{Task: tasks[i], Team: teams[a], Distance: dist(a, i)})
			taken[i] = true
		}
	}
	slices.SortFunc(p.Assign, func(a, b salvageAssignment) int { return int(a.Task.ID - b.Task.ID) })
	for i, t := range tasks {
		if !taken[i] {
			p.Waiting = append(p.Waiting, t)
		}
	}
	return p, nil
}

// readSalvagePlan gathers what planSalvage needs from the store and plans.
func readSalvagePlan(ctx context.Context, q store.Reader) (salvagePlan, error) {
	tasks, err := store.PendingTasks(ctx, q, salvageTask)
	if err != nil {
		return salvagePlan{}, err
	}
	idle, err := store.IdleMiningTeams(ctx, q)
	if err != nil {
		return salvagePlan{}, err
	}
	maxCapacity, err := store.MaxTransportCapacity(ctx, q)
	if err != nil {
		return salvagePlan{}, err
	}
	var teams []store.MiningTeam
	invalid := map[string]string{}
	for _, t := range idle {
		problem, err := salvageTeamProblem(t, maxCapacity)
		if err != nil {
			return salvagePlan{}, err
		}
		if problem != "" {
			invalid[t.Controller+"@"+t.Location] = problem
			continue
		}
		teams = append(teams, t)
	}
	pos := map[string]model.Position{}
	stars := make([]string, 0, len(tasks)+len(teams))
	for _, t := range tasks {
		stars = append(stars, auto.StarOf(t.Location))
	}
	for _, t := range teams {
		stars = append(stars, t.Star)
	}
	for _, s := range stars {
		if _, ok := pos[s]; ok {
			continue
		}
		if pos[s], err = store.StarPosition(ctx, q, s); err != nil {
			return salvagePlan{}, err
		}
	}
	p, err := planSalvage(tasks, teams, pos)
	p.Invalid = invalid
	return p, err
}

// assignSalvage plans (readSalvagePlan) and assigns, returning the tasks it
// assigned.
func assignSalvage(ctx context.Context, tx *sql.Tx, w io.Writer) ([]int64, error) {
	p, err := readSalvagePlan(ctx, tx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var ids []int64
	for _, a := range p.Assign {
		if err := store.AssignTask(ctx, tx, a.Task.ID, a.Team.Controller, now); err != nil {
			return nil, err
		}
		ids = append(ids, a.Task.ID)
	}
	p.print(w)
	return ids, nil
}

// assignSalvageDB is assignSalvage in a transaction of its own.
func assignSalvageDB(ctx context.Context, db *sql.DB, w io.Writer) (ids []int64, err error) {
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		ids, err = assignSalvage(ctx, tx, w)
		return err
	})
	return ids, err
}

func (p salvagePlan) print(w io.Writer) {
	for _, team := range slices.Sorted(maps.Keys(p.Invalid)) {
		fmt.Fprintf(w, "WARNING: mining team %s can't salvage: it %s\n", team, p.Invalid[team])
	}
	fmt.Fprintf(w, "salvage tasks: %d free teams, %d to assign, %d left pending\n", p.Teams, len(p.Assign), len(p.Waiting))
	for _, a := range p.Assign {
		fmt.Fprintf(w, "  %s %d %s <- %s@%s (%d mining_drones, maintenance_drone %s, %.1f ly)\n",
			salvageTask, a.Task.ID, a.Task.Location, a.Team.Controller, a.Team.Location, a.Team.Miners, a.Team.Maint, a.Distance)
	}
}

// --- the handler ---

// salvage is the salvage task's handler: t.Assignee is an
// ami_mining_controller, which with its mining_drones and maintenance_drone
// collects the salvage at t.Location. Each call takes one step
// (decideSalvage), on the team as the API has it now.
func salvage(ctx context.Context, db store.Reader, t store.Task) (taskChange, error) {
	c, err := fetchMember(ctx, t.Assignee, "ami_mining_controller", "")
	if err != nil {
		return nil, err
	}
	maint, ok, err := store.MaintenanceDroneOf(ctx, db, t.Assignee)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s@%s has no maintenance_drone in adopted_maintenance_drones", c.Code, c.Location)
	}
	m, err := fetchMember(ctx, maint, "maintenance_drone", "")
	if err != nil {
		return nil, err
	}
	s := salvageState{Target: t.Location, Phase: t.Phase, Controller: c, Maint: m}
	controlled, err := store.ControlledBy(ctx, db, t.Assignee)
	if err != nil {
		return nil, err
	}
	for _, code := range controlled {
		if code == maint {
			continue
		}
		d, err := fetchMember(ctx, code, "mining_drone", t.Assignee)
		if err != nil {
			return nil, err
		}
		s.Miners = append(s.Miners, d)
	}
	star := auto.StarOf(t.Location)
	if s.TargetEntry, err = store.EntryPoint(ctx, db, star); err != nil {
		return nil, err
	}
	if c.Location != "" && auto.StarOf(c.Location) != star {
		if s.HomeEntry, err = store.EntryPoint(ctx, db, auto.StarOf(c.Location)); err != nil {
			return nil, err
		}
	}
	a, err := decideSalvage(s)
	if err != nil {
		return nil, err
	}
	log.Printf("task %d %s %s (%s@%s): %s", t.ID, t.Task, t.Location, c.Code, c.Location, a.Why)

	switch a.Step {
	case salvageWait:
		return nil, nil
	case salvageClear:
		return nil, api.ClearDirective(ctx, a.Device)
	case salvageStart:
		if a.Patrol {
			if err := api.Patrol(ctx, m.Code); err != nil {
				return nil, err
			}
		}
		if err := api.Salvage(ctx, c.Code, t.Location); err != nil {
			return nil, err
		}
		return setPhase(t, phaseSalvaging), nil
	case salvageGather:
		for _, d := range a.Travellers {
			if err := api.Travel(ctx, d, a.Dest); err != nil {
				return nil, err
			}
		}
		return setPhase(t, phaseGathering), nil
	case salvageDone:
		return func(ctx context.Context, tx *sql.Tx) ([]int64, error) {
			return finishAndNext(ctx, tx, t, c.Code, "ami_mining_controller", star)
		}, nil
	case salvageBlock:
		return blockOnMove(t, s.HomeEntry, s.TargetEntry, s.codes()), nil
	}
	return nil, fmt.Errorf("unknown step %d", a.Step)
}

// salvage phases: gathering at the entry point of the system the team is in
// (to be picked up), then salvaging. While a move brings the team over, the
// task is blocked.
const phaseSalvaging = "salvaging"

type salvageStep int

const (
	salvageWait   salvageStep = iota // nothing to do until something happens
	salvageClear                     // clear Device's directive
	salvageStart                     // (patrol, if Patrol, and) send gather_salvage
	salvageGather                    // send Travellers to Dest
	salvageDone                      // finish the task, find the team another
	salvageBlock                     // wait at the home entry point for a move_devices task
)

type salvageAction struct {
	Step       salvageStep
	Device     string   // salvageClear: whose directive
	Patrol     bool     // salvageStart: put the maintenance_drone on patrol first
	Travellers []string // salvageGather: who to send
	Dest       string   // salvageGather: where to
	Why        string   // for the log
}

type salvageState struct {
	Target      string // the planet or moon to salvage
	TargetEntry string // its star's entry point
	HomeEntry   string // the entry point of the controller's star, if that isn't Target's
	Phase       string // the task's
	Controller  member
	Maint       member
	Miners      []member
}

func (s salvageState) team() []member {
	return slices.Concat([]member{s.Controller, s.Maint}, s.Miners)
}

func (s salvageState) codes() []string {
	var codes []string
	for _, m := range s.team() {
		codes = append(codes, m.Code)
	}
	return codes
}

// decideSalvage is the salvage task's next step:
//
//	controller travelling or carried? wait
//	controller running gather_salvage we started (phase salvaging)? wait
//	controller running anything else? clear it (we take the team over)
//	outside the target system with the maintenance_drone patrolling? clear that
//	anyone else with no location? wait until they arrive
//	in the target system:
//	  we started gather_salvage and it's completed: done
//	  else patrol (if not already) and start gather_salvage
//	elsewhere: anyone away from our own entry point? send them there
//	  else file a move_devices task and wait for it
//
// The team needn't be at the site: the drones go there themselves. Anything
// unexpected is an error: a team split across systems, a member attached to a
// carrier when it must act, gather_salvage ending other than completed, or the
// maintenance_drone running a directive other than patrol.
func decideSalvage(s salvageState) (salvageAction, error) {
	c, m := s.Controller, s.Maint
	target := auto.StarOf(s.Target)
	if c.Location == "" {
		return salvageAction{Step: salvageWait, Why: fmt.Sprintf("waiting: controller is %s with no location (destination %q)", c.Status, c.Destination)}, nil
	}
	star := auto.StarOf(c.Location)
	if c.Directive == salvageDirective && s.Phase == phaseSalvaging {
		if star != target {
			return salvageAction{}, fmt.Errorf("controller %s@%s is running %s for this task, outside %s", c.Code, c.Location, salvageDirective, target)
		}
		return salvageAction{Step: salvageWait, Why: "waiting: " + salvageDirective + " under way"}, nil
	}
	if c.Directive != "" {
		return salvageAction{Step: salvageClear, Device: c.Code, Why: fmt.Sprintf("taking over: clearing controller's %s", c.Directive)}, nil
	}
	if m.Directive != "" && m.Directive != patrolDirective {
		return salvageAction{}, fmt.Errorf("maintenance_drone %s@%s is running %s", m.Code, m.Location, m.Directive)
	}
	if star != target && m.Directive == patrolDirective {
		return salvageAction{Step: salvageClear, Device: m.Code, Why: fmt.Sprintf("leaving %s: clearing maintenance_drone %s's patrol", star, m.Code)}, nil
	}
	for _, d := range s.team() {
		if d.Location == "" {
			return salvageAction{Step: salvageWait, Why: fmt.Sprintf("waiting: %s is %s with no location (destination %q)", d.Code, d.Status, d.Destination)}, nil
		}
	}
	for _, d := range s.team() {
		if auto.StarOf(d.Location) != star {
			return salvageAction{}, fmt.Errorf("team split: controller %s is at %s, but %s is at %s", c.Code, c.Location, d.Code, d.Location)
		}
	}

	if star == target {
		for _, d := range s.team() {
			if d.AttachedTo != "" {
				return salvageAction{}, fmt.Errorf("%s@%s is still attached to %s in the target system", d.Code, d.Location, d.AttachedTo)
			}
		}
		if s.Phase == phaseSalvaging {
			if c.DirectiveName == salvageDirective && c.DirectiveStatus == "completed" {
				return salvageAction{Step: salvageDone, Why: salvageDirective + " completed"}, nil
			}
			return salvageAction{}, fmt.Errorf("controller %s@%s: %s ended, but its directive is %q, status %q (cleared?)",
				c.Code, c.Location, salvageDirective, c.DirectiveName, c.DirectiveStatus)
		}
		why := "starting " + salvageDirective
		if m.Directive != patrolDirective {
			why = "patrolling and " + why
		}
		return salvageAction{Step: salvageStart, Patrol: m.Directive != patrolDirective, Why: why}, nil
	}

	if s.HomeEntry == "" {
		return salvageAction{}, fmt.Errorf("no entry point for %s", star)
	}
	away := awayFrom(s.team(), s.HomeEntry)
	if len(away) == 0 {
		return salvageAction{Step: salvageBlock,
			Why: fmt.Sprintf("team at %s; filing move_devices to %s", s.HomeEntry, s.TargetEntry)}, nil
	}
	for _, d := range s.team() {
		if d.AttachedTo != "" && slices.Contains(away, d.Code) {
			return salvageAction{}, fmt.Errorf("%s is attached to %s at %s, away from %s", d.Code, d.AttachedTo, d.Location, s.HomeEntry)
		}
	}
	return salvageAction{Step: salvageGather, Travellers: away, Dest: s.HomeEntry,
		Why: fmt.Sprintf("not in %s; gathering at %s: sending %s", target, s.HomeEntry, strings.Join(away, " "))}, nil
}
