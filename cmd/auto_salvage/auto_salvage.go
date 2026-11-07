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
	for {
		star, err := salvageStar(ctx, hub)
		if err != nil {
			log.Fatal(err)
		}
		next, site, err := busiestStar(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if next == "" {
			log.Printf("no salvage left at any known star; done")
			return
		}
		if next == star {
			log.Fatalf("%s still has salvage at %s after salvaging it", star, site)
		}
		log.Printf("moving transport %s from %s to %s (%s)", transportID, star, next, site)
		if err := auto.GoTo(ctx, hub, transportID, site); err != nil {
			log.Fatal(err)
		}
	}
}

// salvageStar runs one full salvage cycle at the transport's current star: detach
// the miners, salvage every site in the system, then re-attach them. It returns the star.
func salvageStar(ctx context.Context, hub *api.Hub) (string, error) {
	t, err := api.Device(ctx, transportID)
	if err != nil {
		return "", err
	}
	if false {
		fmt.Printf("Transport %s\n", litter.Sdump(t))
	}

	star, _, _ := strings.Cut(t.Location, "-")
	if star == "" {
		return "", fmt.Errorf("transport %s has no location (in transit?)", transportID)
	}

	// Detach any attached mining drones, controller and maintenance drones.
	miners, maint := splitAttached(t.AttachedDevices)
	targets := slices.Concat(miners, maint)
	log.Printf("detaching miners %s, maintenance drones %s", miners, maint)
	if len(targets) > 0 {
		if err := api.DetachDevice(ctx, transportID, targets...); err != nil {
			return "", err
		}
	}
	for _, m := range maint {
		if err := api.Patrol(ctx, m); err != nil {
			return "", fmt.Errorf("patrol %s: %w", m, err)
		}
		log.Printf("maintenance drone %s patrolling %s", m, star)
	}
	controller, err := adoptMinersAtStar(ctx, star)
	if err != nil {
		return "", err
	}
	log.Printf("controller: %s", controller)

	for {
		site, err := findSalvageSite(ctx, star)
		if err != nil {
			return "", err
		}
		if site == "" {
			log.Printf("no salvage left at %s", star)
			break
		}
		log.Printf("salvage site: %s", site)

		// Register interest before starting the directive, so a fast completion isn't missed.
		done := hub.Expect(api.AnyOf(
			api.ForDevice[model.DirectiveCompleted](controller),
			api.ForDevice[model.DirectiveCleared](controller),
		))
		if err := api.Salvage(ctx, controller, site); err != nil {
			done.Cancel()
			return "", err
		}
		log.Printf("waiting for %s to finish salvaging %s", controller, site)
		ev, err := done.Wait(ctx)
		if err != nil {
			return "", err
		}
		switch p := ev.Payload.(type) {
		case *model.DirectiveCompleted:
			log.Printf("%s completed %s at %s", controller, p.Directive, site)
		case *model.DirectiveCleared:
			return "", fmt.Errorf("%s directive %s was cleared before completing", controller, p.PreviousDirective)
		}
	}

	return star, reattach(ctx, hub, controller, star, maint)
}

// splitAttached picks out the attached devices we detach at each star: the
// mining drones and controller, and the maintenance drones.
func splitAttached(attached []model.AttachedDevice) (miners, maint []string) {
	for _, d := range attached {
		switch d.DeviceType {
		case "mining_drone", "ami_mining_controller":
			miners = append(miners, d.DeviceCode)
		case "maintenance_drone":
			maint = append(maint, d.DeviceCode)
		}
	}
	return miners, maint
}

// reattach brings the controller, the mining drones it controls and the
// maintenance drones (taken off patrol) back to the transport, and attaches them to it.
func reattach(ctx context.Context, hub *api.Hub, controller, star string, maint []string) error {
	t, err := api.Device(ctx, transportID)
	if err != nil {
		return err
	}
	drones, err := auto.DevicesInSystem(ctx, star, "mining_drone")
	if err != nil {
		return err
	}
	devices := []string{controller}
	for _, d := range drones {
		if d.ControllerDeviceCode == controller {
			devices = append(devices, d.DeviceCode)
		}
	}
	for _, m := range maint {
		if err := api.ClearDirective(ctx, m); err != nil {
			return fmt.Errorf("stop %s patrolling: %w", m, err)
		}
		log.Printf("maintenance drone %s off patrol", m)
	}
	devices = append(devices, maint...)
	log.Printf("returning %s to transport %s at %s", devices, transportID, t.Location)
	if err := auto.GoToAll(ctx, hub, devices, t.Location); err != nil {
		return err
	}
	if err := api.AttachDevice(ctx, transportID, devices...); err != nil {
		return fmt.Errorf("attach to %s: %w", transportID, err)
	}
	log.Printf("attached %d device(s) to %s", len(devices), transportID)
	return nil
}

// isSalvageSite reports whether loc is somewhere we salvage: it has resource
// sites and isn't an asteroid belt or an object. SOL never counts: it's far
// from every other star, and we can't salvage there anyway.
func isSalvageSite(loc string, summary model.LocationSummary) bool {
	return summary.ResourceSites > 0 &&
		!strings.HasPrefix(loc, "SOL-") &&
		!strings.Contains(loc, "-BELT-") &&
		!strings.Contains(loc, "-OBJ-")
}

// findSalvageSite returns an arbitrary (alphabetically first) salvage site at the star.
// It returns "" (and no error) if there are none left.
func findSalvageSite(ctx context.Context, star string) (string, error) {
	list, err := api.Locations(ctx)
	if err != nil {
		return "", err
	}
	var sites []string
	for loc, summary := range list.Locations {
		if strings.HasPrefix(loc, star+"-") && isSalvageSite(loc, summary) {
			sites = append(sites, loc)
		}
	}
	if len(sites) == 0 {
		return "", nil
	}
	slices.Sort(sites)
	log.Printf("salvage sites at %s: %s", star, sites)
	return sites[0], nil
}

// busiestStar returns the star with the most resource sites across its salvage
// sites, and one of those sites (alphabetically first) to travel to.
// It returns "" (and no error) if no known star has any salvage.
func busiestStar(ctx context.Context) (star, site string, err error) {
	list, err := api.Locations(ctx)
	if err != nil {
		return "", "", err
	}
	star, site = pickBusiestStar(list.Locations)
	return star, site, nil
}

func pickBusiestStar(locs map[string]model.LocationSummary) (star, site string) {
	counts := map[string]int{}
	first := map[string]string{}
	for loc, summary := range locs {
		if !isSalvageSite(loc, summary) {
			continue
		}
		s, _, _ := strings.Cut(loc, "-")
		counts[s] += summary.ResourceSites
		if f, ok := first[s]; !ok || loc < f {
			first[s] = loc
		}
	}
	best := -1
	for s, n := range counts {
		// Ties go to the alphabetically first star, so the choice is stable.
		if n > best || (n == best && s < star) {
			star, best = s, n
		}
	}
	if star != "" {
		log.Printf("busiest star: %s (%d resource sites)", star, best)
	}
	return star, first[star]
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
