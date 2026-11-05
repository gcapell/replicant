package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
	"github.com/sanity-io/litter"
)

var transportID = "E62C8DC1" // drones are attached to this.

func main() {
	ctx := context.Background()
	hub := api.NewHub(ctx)
	t, err := api.Device(ctx, transportID)
	if err != nil {
		log.Fatal(err)
	}
	if false {
		fmt.Printf("Transport %s\n", litter.Sdump(t))
	}

	// Detach any attached mining drones.
	var targets []string
	for _, d := range t.AttachedDevices {
		switch d.DeviceType {
		case "mining_drone", "ami_mining_controller":
			targets = append(targets, d.DeviceCode)
		}
	}
	log.Printf("targets: %s", targets)
	if len(targets) > 0 {
		if err := api.DetachDevice(ctx, transportID, targets...); err != nil {
			log.Fatal(err)
		}
	}
	star, _, _ := strings.Cut(t.Location, "-")
	if star == "" {
		log.Fatalf("transport %s has no location (in transit?)", transportID)
	}
	controller, err := adoptMinersAtStar(ctx, star)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("controller: %s", controller)

	for {
		// find an arbitrary salvage site at this star (use /locations API)
		site, err := findSalvageSite(ctx, star)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("salvage site: %s", site)

		// Register interest before starting the directive, so a fast completion isn't missed.
		done := hub.Expect(api.AnyOf(
			api.ForDevice[model.DirectiveCompleted](controller),
			api.ForDevice[model.DirectiveCleared](controller),
		))
		if err := api.Salvage(ctx, controller, site); err != nil {
			done.Cancel()
			log.Fatal(err)
		}
		log.Printf("waiting for %s to finish salvaging %s", controller, site)
		ev, err := done.Wait(ctx)
		if err != nil {
			log.Fatal(err)
		}
		switch p := ev.Payload.(type) {
		case *model.DirectiveCompleted:
			log.Printf("%s completed %s at %s", controller, p.Directive, site)
		case *model.DirectiveCleared:
			log.Fatalf("%s directive %s was cleared before completing", controller, p.PreviousDirective)
		}
	}
}

// findSalvageSite returns an arbitrary (alphabetically first) salvage site at the star:
// a location in the system with resource sites that isn't an asteroid belt.
func findSalvageSite(ctx context.Context, star string) (string, error) {
	list, err := api.Locations(ctx)
	if err != nil {
		return "", err
	}
	var sites []string
	for loc, summary := range list.Locations {
		if strings.HasPrefix(loc, star+"-") &&
			summary.ResourceSites > 0 &&
			!strings.Contains(loc, "-BELT-") &&
			!strings.Contains(loc, "-OBJ-") {
			sites = append(sites, loc)
		}
	}
	if len(sites) == 0 {
		return "", fmt.Errorf("no salvage site at %s", star)
	}
	slices.Sort(sites)
	log.Printf("salvage sites at %s: %s", star, sites)
	return sites[0], nil
}

// adoptMinersAtStar finds the first ami_mining_controller in the star system with no
// directive (or a completed one), and has it adopt every mining_drone in the system
// that has no controller. It returns the selected controller's device code.
func adoptMinersAtStar(ctx context.Context, star string) (string, error) {
	drones, err := auto.DevicesInSystem(ctx, star, "mining_drone")
	if err != nil {
		return "", err
	}
	controllers, err := auto.DevicesInSystem(ctx, star, "ami_mining_controller")
	if err != nil {
		return "", err
	}
	log.Printf("%s: %d mining_drone, %d ami_mining_controller", star, len(drones), len(controllers))

	var controller *model.Device
	for i, c := range controllers {
		if c.AmiDirective.Name == "" ||
			c.AmiDirective.Name == "gather_salvage" ||
			c.AmiDirectiveStatus == "completed" {
			controller = &controllers[i]
			break
		}
	}
	if controller == nil {
		return "", fmt.Errorf("no idle ami_mining_controller in %s", star)
	}
	log.Printf("controller: %s (directive %q, status %q)",
		controller.DeviceCode, controller.AmiDirective.Name, controller.AmiDirectiveStatus)

	var orphans []string
	for _, d := range drones {
		if d.ControllerDeviceCode == "" {
			orphans = append(orphans, d.DeviceCode)
		}
	}
	log.Printf("uncontrolled drones: %s", orphans)
	if len(orphans) > 0 {
		if err := api.AdoptDevice(ctx, controller.DeviceCode, orphans...); err != nil {
			return "", err
		}
	}
	return controller.DeviceCode, nil
}
