package main

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gcapell/replicant/model"
)

// teamMiners is how many mining_drones a team takes: a surge_carrier attaches
// 9 devices, and the rest of the team is 4.
const teamMiners = 5

// carrierCapacity is how many devices each kind of carrier can attach.
// Hardcoded: the store doesn't record attach_capacity.
var carrierCapacity = map[string]int{
	"surge_plate":    1,
	"surge_platform": 4,
	"surge_carrier":  9,
}

// preferred resources count first when ranking.
var preferred = map[model.ResourceType]bool{model.Volatiles: true, model.Rares: true}

var densities = map[string]int{"sparse": 0, "moderate": 1, "dense": 2}

// levels, worst first.
var levels = []string{"scarce", "low", "moderate", "high", "rich"}

// Belt is an asteroid belt as the store has it.
type Belt struct {
	Designation string
	Star        string
	Pos         model.Position
	Density     string
	Levels      map[model.ResourceType]string
}

// Device is a device as the store has it. Location is "" while travelling.
type Device struct {
	Code, Type, Status            string
	Location, Star                string
	AttachedTo, StowedIn, Control string
	Destination                   string
}

func (d Device) free() bool {
	return d.Status == "idle" && d.Location != "" && d.AttachedTo == "" && d.StowedIn == ""
}

// Team is the devices that will start a belt, all at Location.
type Team struct {
	Location      string
	Star          string
	Pos           model.Position
	MiningControl string
	SurveyControl string
	Surveyor      string
	Maintenance   string
	Miners        []string
}

// Devices lists every device in the team.
func (t Team) Devices() []string {
	return append([]string{t.MiningControl, t.SurveyControl, t.Surveyor, t.Maintenance}, t.Miners...)
}

// Carrier is the surge_carrier taking a team to its belt.
type Carrier struct {
	Code, Location string
	Dist           float64 // light years from its star to the team's
}

// Plan is everything start_belt will do.
type Plan struct {
	Belts   []Belt // candidates, best first
	Belt    Belt
	Team    Team
	Carrier Carrier
	Dist    float64 // from the team's star to the belt's
}

// developedBelts are the belts with an ami_mining_controller at them or on
// its way there.
func developedBelts(devices []Device) map[string]bool {
	m := map[string]bool{}
	for _, d := range devices {
		if d.Type == "ami_mining_controller" {
			for _, loc := range []string{d.Location, d.Destination} {
				if strings.Contains(loc, "-BELT-") {
					m[loc] = true
				}
			}
		}
	}
	return m
}

// candidates returns the belts at relay stars that nothing is mining yet.
func candidates(belts []Belt, devices []Device, relayStars map[string]bool) []Belt {
	developed := developedBelts(devices)
	var out []Belt
	for _, b := range belts {
		if relayStars[b.Star] && !developed[b.Designation] {
			out = append(out, b)
		}
	}
	return out
}

// score orders belts: higher is better, compared element by element. Density
// first, then, from the best level down, how many preferred resources and how
// many others are at that level.
func score(b Belt) ([]int, error) {
	d, ok := densities[b.Density]
	if !ok {
		return nil, fmt.Errorf("belt %s: unknown density %q", b.Designation, b.Density)
	}
	s := []int{d}
	count := make([][2]int, len(levels))
	for _, r := range model.Resources {
		l := slices.Index(levels, b.Levels[r])
		if l < 0 {
			return nil, fmt.Errorf("belt %s: unknown %s level %q", b.Designation, r, b.Levels[r])
		}
		if preferred[r] {
			count[l][0]++
		} else {
			count[l][1]++
		}
	}
	for l := len(levels) - 1; l > 0; l-- {
		s = append(s, count[l][0], count[l][1])
	}
	return s, nil
}

// teams sorts every location's free devices into a team: first the
// complete ones (by location), then the rest, closest to complete first.
func teams(devices []Device) (complete, partial []Team) {
	byLoc := map[string][]Device{}
	for _, d := range devices {
		if d.free() {
			byLoc[d.Location] = append(byLoc[d.Location], d)
		}
	}
	for loc, ds := range byLoc {
		slices.SortFunc(ds, func(a, b Device) int { return cmp.Compare(a.Code, b.Code) })
		t := Team{Location: loc, Star: ds[0].Star}
		for _, d := range ds {
			switch {
			case d.Type == "ami_mining_controller" && t.MiningControl == "":
				t.MiningControl = d.Code
			case d.Type == "ami_survey_controller" && t.SurveyControl == "":
				t.SurveyControl = d.Code
			case d.Type == "belt_surveyor" && d.Control == "" && t.Surveyor == "":
				t.Surveyor = d.Code
			case d.Type == "maintenance_drone" && t.Maintenance == "":
				t.Maintenance = d.Code
			case d.Type == "mining_drone" && d.Control == "" && len(t.Miners) < teamMiners:
				t.Miners = append(t.Miners, d.Code)
			}
		}
		if t.complete() {
			complete = append(complete, t)
		} else if t.size() > 0 {
			partial = append(partial, t)
		}
	}
	slices.SortFunc(complete, func(a, b Team) int { return cmp.Compare(a.Location, b.Location) })
	slices.SortFunc(partial, func(a, b Team) int {
		return cmp.Or(cmp.Compare(b.size(), a.size()), cmp.Compare(a.Location, b.Location))
	})
	return complete, partial
}

// roles are a team's single members, in the order we show them.
func (t Team) roles() []struct{ typ, code string } {
	return []struct{ typ, code string }{
		{"ami_mining_controller", t.MiningControl},
		{"ami_survey_controller", t.SurveyControl},
		{"belt_surveyor", t.Surveyor},
		{"maintenance_drone", t.Maintenance},
	}
}

// size is how many of the team's places are filled.
func (t Team) size() int {
	n := len(t.Miners)
	for _, r := range t.roles() {
		if r.code != "" {
			n++
		}
	}
	return n
}

func (t Team) complete() bool { return t.size() == len(t.roles())+teamMiners }

// shortfall describes what a team has and lacks, like
// "has ami_mining_controller, belt_surveyor; lacks ami_survey_controller,
// maintenance_drone; 3/5 mining_drones".
func (t Team) shortfall() string {
	var has, lacks []string
	for _, r := range t.roles() {
		if r.code != "" {
			has = append(has, r.typ)
		} else {
			lacks = append(lacks, r.typ)
		}
	}
	var parts []string
	if len(has) > 0 {
		parts = append(parts, "has "+strings.Join(has, ", "))
	}
	if len(lacks) > 0 {
		parts = append(parts, "lacks "+strings.Join(lacks, ", "))
	}
	parts = append(parts, fmt.Sprintf("%d/%d free mining_drones", len(t.Miners), teamMiners))
	return strings.Join(parts, "; ")
}

// closest is how many of the nearest-complete teams a failure lists.
const closest = 3

func noTeamError(partial []Team) error {
	msg := fmt.Sprintf("no location has a free team (ami_mining_controller, ami_survey_controller, belt_surveyor, maintenance_drone, %d mining_drones; all idle, unattached and unstowed, the drones and surveyor with no controller)", teamMiners)
	if len(partial) == 0 {
		return fmt.Errorf("%s, nor any part of one", msg)
	}
	msg += ". Closest:"
	for _, t := range partial[:min(closest, len(partial))] {
		msg += fmt.Sprintf("\n  %s: %s", t.Location, t.shortfall())
	}
	return errors.New(msg)
}

// makePlan picks the best belt, the team nearest it, and the surge_carrier
// nearest the team. starPos has the positions of the stars we know.
func makePlan(belts []Belt, devices []Device, relayStars map[string]bool, starPos map[string]model.Position) (*Plan, error) {
	cands := candidates(belts, devices, relayStars)
	if len(cands) == 0 {
		return nil, fmt.Errorf("no undeveloped belts at relay stars")
	}
	ts, partial := teams(devices)
	if len(ts) == 0 {
		return nil, noTeamError(partial)
	}
	for i, t := range ts {
		p, ok := starPos[t.Star]
		if !ok {
			return nil, fmt.Errorf("team at %s: no position for star %s", t.Location, t.Star)
		}
		ts[i].Pos = p
	}
	nearest := func(b Belt) (Team, float64) {
		best := slices.MinFunc(ts, func(x, y Team) int {
			return cmp.Compare(x.Pos.Distance(b.Pos), y.Pos.Distance(b.Pos))
		})
		return best, best.Pos.Distance(b.Pos)
	}

	scores := map[string][]int{}
	for _, b := range cands {
		s, err := score(b)
		if err != nil {
			return nil, err
		}
		scores[b.Designation] = s
	}
	slices.SortFunc(cands, func(a, b Belt) int {
		if c := slices.Compare(scores[b.Designation], scores[a.Designation]); c != 0 {
			return c
		}
		_, da := nearest(a)
		_, db := nearest(b)
		return cmp.Or(cmp.Compare(da, db), cmp.Compare(a.Designation, b.Designation))
	})

	p := &Plan{Belts: cands, Belt: cands[0]}
	p.Team, p.Dist = nearest(p.Belt)

	var carriers []Carrier
	for _, d := range devices {
		if d.Type != "surge_carrier" || !d.free() {
			continue
		}
		pos, ok := starPos[d.Star]
		if !ok {
			return nil, fmt.Errorf("surge_carrier %s at %s: no position for star %s", d.Code, d.Location, d.Star)
		}
		carriers = append(carriers, Carrier{Code: d.Code, Location: d.Location, Dist: pos.Distance(p.Team.Pos)})
	}
	if len(carriers) == 0 {
		return nil, fmt.Errorf("no idle surge_carrier")
	}
	if n := len(p.Team.Devices()); n > carrierCapacity["surge_carrier"] {
		return nil, fmt.Errorf("team of %d won't fit a surge_carrier (%d)", n, carrierCapacity["surge_carrier"])
	}
	// Nearest first; one already at the team's location beats others at its star.
	p.Carrier = slices.MinFunc(carriers, func(a, b Carrier) int {
		return cmp.Or(
			cmp.Compare(a.Dist, b.Dist),
			cmp.Compare(boolInt(a.Location != p.Team.Location), boolInt(b.Location != p.Team.Location)),
			cmp.Compare(a.Code, b.Code),
		)
	})
	return p, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
