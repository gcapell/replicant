// auto_scan plans how survey teams will scan the planets and moons of every
// system that has an active ftl_relay. A survey team is a transport (with
// surge and attach) carrying one ami_survey_controller and at least three
// survey_drones that it controls.
//
// It checks each team, finds the systems with unscanned planets or moons,
// spreads them among the teams (balancing the work), and prints each team's
// route. Only the dry run (-n) is implemented so far.
//
//	auto_scan -n TRANSPORT_ID...
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

const minDrones = 3

func main() {
	dryRun := flag.Bool("n", false, "print the plan only")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-n] TRANSPORT_ID...\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(context.Background(), flag.Args(), *dryRun); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, transports []string, dryRun bool) error {
	teams, err := validateTeams(ctx, transports)
	if err != nil {
		return err
	}
	systems, err := unscannedSystems(ctx)
	if err != nil {
		return err
	}
	if err := locateTeams(ctx, teams, systems); err != nil {
		return err
	}
	routes := plan(teams, systems)
	for _, line := range describe(routes) {
		fmt.Println(line)
	}
	if !dryRun {
		return errors.New("scanning isn't implemented yet; use -n")
	}
	return nil
}

// validateTeams checks every transport and reports all the problems at once.
func validateTeams(ctx context.Context, transports []string) ([]team, error) {
	var (
		teams []team
		errs  []error
		seen  = map[string]bool{}
	)
	for _, id := range transports {
		if seen[id] {
			errs = append(errs, fmt.Errorf("%s: listed twice", id))
			continue
		}
		seen[id] = true
		t, err := validateTeam(ctx, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		log.Printf("team %s at %s: controller %s, %d drones", t.Transport, t.Star, t.Controller, len(t.Drones))
		teams = append(teams, t)
	}
	return teams, errors.Join(errs...)
}

func validateTeam(ctx context.Context, id string) (team, error) {
	tr, err := api.Device(ctx, id)
	if err != nil {
		return team{}, fmt.Errorf("%s: %w", id, err)
	}
	t, err := checkTransport(tr)
	if err != nil {
		return team{}, err
	}
	for _, d := range t.Drones {
		drone, err := api.Device(ctx, d)
		if err != nil {
			return team{}, fmt.Errorf("%s: drone %s: %w", id, d, err)
		}
		if drone.ControllerDeviceCode != t.Controller {
			return team{}, fmt.Errorf("%s: drone %s is controlled by %q, not %s",
				id, d, drone.ControllerDeviceCode, t.Controller)
		}
	}
	return t, nil
}

// checkTransport checks what we can see from the transport itself: its
// features, status and what's attached to it.
func checkTransport(tr *model.Device) (team, error) {
	id := tr.DeviceCode
	for _, f := range []string{"surge", "attach"} {
		if !slices.Contains(tr.Features, f) {
			return team{}, fmt.Errorf("%s (%s) has no %s feature", id, tr.DeviceType, f)
		}
	}
	if tr.Status != "idle" {
		return team{}, fmt.Errorf("%s is %s, not idle", id, tr.Status)
	}
	t := team{Transport: id, Star: auto.StarOf(tr.Location)}
	var controllers, others []string
	for _, a := range tr.AttachedDevices {
		switch a.DeviceType {
		case "ami_survey_controller":
			controllers = append(controllers, a.DeviceCode)
		case "survey_drone":
			t.Drones = append(t.Drones, a.DeviceCode)
		default:
			others = append(others, a.DeviceCode+" ("+a.DeviceType+")")
		}
	}
	switch {
	case len(controllers) != 1:
		return team{}, fmt.Errorf("%s has %d ami_survey_controllers attached, want 1", id, len(controllers))
	case len(t.Drones) < minDrones:
		return team{}, fmt.Errorf("%s has %d survey_drones attached, want at least %d", id, len(t.Drones), minDrones)
	case len(others) > 0:
		return team{}, fmt.Errorf("%s also has %s attached", id, strings.Join(others, ", "))
	}
	t.Controller = controllers[0]
	return t, nil
}

// unscannedSystems returns the systems with an active relay that still have
// unscanned planets or moons.
func unscannedSystems(ctx context.Context) ([]system, error) {
	stars, err := relayStars(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("%d systems with an active relay", len(stars))
	var systems []system
	for _, star := range stars {
		loc, err := api.Location(ctx, star)
		if err != nil {
			return nil, err
		}
		s := toSystem(star, loc)
		switch {
		case !loc.SystemScanned:
			log.Printf("%s: system not scanned yet; skipping it", star)
		case s.Unscanned > 0:
			systems = append(systems, s)
		}
	}
	log.Printf("%d of them need scanning", len(systems))
	return systems, nil
}

func toSystem(star string, loc *model.Location) system {
	p := loc.PlanetsTotal - loc.PlanetsScanned
	m := loc.MoonsTotal - loc.MoonsScanned
	return system{Name: star, Pos: loc.Star.Position, Planets: p, Moons: m, Unscanned: p + m}
}

// relayStars returns the stars that have a relaying ftl_relay, sorted.
func relayStars(ctx context.Context) ([]string, error) {
	relays, err := auto.Devices(ctx, "ftl_relay")
	if err != nil {
		return nil, err
	}
	var stars []string
	for _, r := range relays {
		if r.Status == "relaying" && r.StowedInDeviceCode == "" {
			stars = append(stars, auto.StarOf(r.Location))
		}
	}
	slices.Sort(stars)
	return slices.Compact(stars), nil
}

// locateTeams fills in each team's position, fetching its star unless it's
// one of systems.
func locateTeams(ctx context.Context, teams []team, systems []system) error {
	for i := range teams {
		if j := slices.IndexFunc(systems, func(s system) bool { return s.Name == teams[i].Star }); j >= 0 {
			teams[i].Pos = systems[j].Pos
			continue
		}
		loc, err := api.Location(ctx, teams[i].Star)
		if err != nil {
			return fmt.Errorf("team %s: %w", teams[i].Transport, err)
		}
		teams[i].Pos = loc.Star.Position
	}
	return nil
}

// describe formats each route as one line, e.g.
//
//	team E62C8DC1 (at MIRETH) -> CORBRENEUS (7.1ly, 3P/5M), PARBEA (3.4ly, 1P/0M)  total 10.5ly, 9 bodies
func describe(routes []route) []string {
	var lines []string
	for _, r := range routes {
		head := fmt.Sprintf("team %s (at %s)", r.Team.Transport, r.Team.Star)
		if len(r.Stops) == 0 {
			lines = append(lines, head+": nothing to do")
			continue
		}
		var stops []string
		at, total, bodies := r.Team.Pos, 0.0, 0
		for _, s := range r.Stops {
			leg := at.Distance(s.Pos)
			total += leg
			bodies += s.Unscanned
			stops = append(stops, fmt.Sprintf("%s (%.1fly, %dP/%dM)", s.Name, leg, s.Planets, s.Moons))
			at = s.Pos
		}
		lines = append(lines, fmt.Sprintf("%s -> %s  total %.1fly, %d bodies",
			head, strings.Join(stops, ", "), total, bodies))
	}
	return lines
}
