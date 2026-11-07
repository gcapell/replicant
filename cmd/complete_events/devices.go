package main

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// Device logistics: carriers (surge_carrier, surge_platform, surge_plate)
// attach spare devices where they are and detach them at the locations that
// want them. This is a multi-vehicle pickup-and-delivery problem; we build a
// plan by greedy insertion and improve it by relocating jobs, aiming first
// for the earliest finish (shortest longest route), then the least total
// travel.

const (
	// inSystemCost is the cost of a move between two locations of one
	// system, in ly-equivalents. FIXME: calibrate travel time (inter-star
	// and in-system) from travel.departed's TravelTimeSeconds.
	inSystemCost = 0.1
	// candidates is how many of the nearest spare devices each job considers.
	candidates = 4
	// maxRounds bounds the local search.
	maxRounds = 50
)

// place is a location and its star's position.
type place struct {
	Loc string
	Pos model.Position
}

// cost estimates travel time from a to b, in ly-equivalents.
func cost(a, b place) float64 {
	switch {
	case a.Loc == b.Loc:
		return 0
	case auto.StarOf(a.Loc) == auto.StarOf(b.Loc):
		return inSystemCost
	}
	return a.Pos.Distance(b.Pos) + inSystemCost
}

// job is one device of Type wanted at Dest.
type job struct {
	Type string
	Dest place
}

// spare is a device available to be moved.
type spare struct {
	Code string
	Type string
	At   place
}

// carrier can attach Free more devices.
type carrier struct {
	Code string
	Type string
	At   place
	Free int
}

// stop is one attach (Pickup) or detach of a spare for a job.
type stop struct {
	At     place
	Job    int
	Spare  int
	Pickup bool
}

// devicePlan holds one route per carrier (same order as the carriers), and
// why each job that isn't in a route was left out.
type devicePlan struct {
	Routes     [][]stop
	Unassigned map[int]string
}

func routeCost(start place, r []stop) float64 {
	t, at := 0.0, start
	for _, s := range r {
		t += cost(at, s.At)
		at = s.At
	}
	return t
}

// fits reports whether r never has more than free devices attached.
func fits(r []stop, free int) bool {
	load := 0
	for _, s := range r {
		if s.Pickup {
			load++
		} else {
			load--
		}
		if load > free {
			return false
		}
	}
	return true
}

// score is (finish time, total time); lower is better.
type score struct{ Max, Sum float64 }

func (a score) less(b score) bool {
	const eps = 1e-9
	if math.Abs(a.Max-b.Max) > eps {
		return a.Max < b.Max
	}
	return a.Sum < b.Sum-eps
}

type planner struct {
	jobs     []job
	spares   []spare
	carriers []carrier
	cands    [][]int // per job: spare indices, nearest first
	routes   [][]stop
	costs    []float64   // per route
	used     map[int]int // spare -> job
	where    map[int]int // job -> carrier
}

func planDevices(jobs []job, spares []spare, carriers []carrier) devicePlan {
	p := &planner{
		jobs: jobs, spares: spares, carriers: carriers,
		routes: make([][]stop, len(carriers)),
		costs:  make([]float64, len(carriers)),
		used:   map[int]int{}, where: map[int]int{},
	}
	out := devicePlan{Unassigned: map[int]string{}}

	// Each job's candidate spares: the nearest few of the right type.
	p.cands = make([][]int, len(jobs))
	for j, jb := range jobs {
		var c []int
		for s, sp := range spares {
			if sp.Type == jb.Type {
				c = append(c, s)
			}
		}
		slices.SortStableFunc(c, func(a, b int) int {
			return cmp.Compare(cost(spares[a].At, jb.Dest), cost(spares[b].At, jb.Dest))
		})
		p.cands[j] = c[:min(len(c), candidates)]
	}

	// Insert the hardest jobs (furthest from any spare) first: they decide
	// the finish time.
	order := make([]int, 0, len(jobs))
	for j := range jobs {
		if len(p.cands[j]) == 0 {
			out.Unassigned[j] = "no spare " + jobs[j].Type
			continue
		}
		order = append(order, j)
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(p.nearest(b), p.nearest(a))
	})
	for _, j := range order {
		if !p.insertBest(j) {
			out.Unassigned[j] = "no carrier with room"
		}
	}

	// Local search: take each job out and put it back wherever is best
	// (possibly with another spare). That never makes things worse; stop
	// when a round improves nothing.
	for range maxRounds {
		improved := false
		for _, j := range order {
			if _, ok := p.where[j]; !ok {
				continue
			}
			before := p.score()
			saved := p.snapshot()
			p.remove(j)
			p.insertBest(j)
			if after := p.score(); after.less(before) {
				improved = true
			} else {
				p.restore(saved)
			}
		}
		if !improved {
			break
		}
	}
	out.Routes = p.routes
	return out
}

func (p *planner) nearest(j int) float64 {
	return cost(p.spares[p.cands[j][0]].At, p.jobs[j].Dest)
}

func (p *planner) score() score {
	var s score
	for _, c := range p.costs {
		s.Max = max(s.Max, c)
		s.Sum += c
	}
	return s
}

// insertBest adds job j, using any free candidate spare, to whichever
// carrier and positions give the best score. It reports whether it could.
func (p *planner) insertBest(j int) bool {
	cur := p.score()
	var (
		best      score
		bestRoute []stop
		bestC     = -1
		bestS     int
	)
	for _, s := range p.cands[j] {
		if _, taken := p.used[s]; taken {
			continue
		}
		pick := stop{At: p.spares[s].At, Job: j, Spare: s, Pickup: true}
		drop := stop{At: p.jobs[j].Dest, Job: j, Spare: s}
		for c, cr := range p.carriers {
			if cr.Free <= 0 {
				continue
			}
			others := 0.0
			for o, oc := range p.costs {
				if o != c {
					others = max(others, oc)
				}
			}
			r := p.routes[c]
			for a := 0; a <= len(r); a++ {
				for b := a; b <= len(r); b++ {
					nr := make([]stop, 0, len(r)+2)
					nr = append(nr, r[:a]...)
					nr = append(nr, pick)
					nr = append(nr, r[a:b]...)
					nr = append(nr, drop)
					nr = append(nr, r[b:]...)
					if !fits(nr, cr.Free) {
						continue
					}
					nc := routeCost(cr.At, nr)
					sc := score{Max: max(others, nc), Sum: cur.Sum - p.costs[c] + nc}
					if bestC < 0 || sc.less(best) {
						best, bestRoute, bestC, bestS = sc, nr, c, s
					}
				}
			}
		}
	}
	if bestC < 0 {
		return false
	}
	p.routes[bestC] = bestRoute
	p.costs[bestC] = routeCost(p.carriers[bestC].At, bestRoute)
	p.used[bestS] = j
	p.where[j] = bestC
	return true
}

func (p *planner) remove(j int) {
	c := p.where[j]
	p.routes[c] = slices.DeleteFunc(slices.Clone(p.routes[c]), func(s stop) bool { return s.Job == j })
	p.costs[c] = routeCost(p.carriers[c].At, p.routes[c])
	for s, jj := range p.used {
		if jj == j {
			delete(p.used, s)
		}
	}
	delete(p.where, j)
}

type snapshot struct {
	routes [][]stop
	costs  []float64
	used   map[int]int
	where  map[int]int
}

func (p *planner) snapshot() snapshot {
	r := make([][]stop, len(p.routes))
	for i := range p.routes {
		r[i] = slices.Clone(p.routes[i])
	}
	return snapshot{r, slices.Clone(p.costs), maps.Clone(p.used), maps.Clone(p.where)}
}

func (p *planner) restore(s snapshot) {
	p.routes, p.costs, p.used, p.where = s.routes, s.costs, s.used, s.where
}

// describeDevices renders the plan: one block per carrier with work, one
// line per location visited, then the jobs left out.
func describeDevices(dp devicePlan, jobs []job, spares []spare, carriers []carrier) []string {
	if len(jobs) == 0 {
		return nil
	}
	finish := 0.0
	for c, r := range dp.Routes {
		finish = max(finish, routeCost(carriers[c].At, r))
	}
	lines := []string{fmt.Sprintf("Device deliveries: %d jobs, finish ~%.1f ly", len(jobs), finish)}
	for c, r := range dp.Routes {
		if len(r) == 0 {
			continue
		}
		cr := carriers[c]
		lines = append(lines, fmt.Sprintf("%s (%s, %d free) from %s, ~%.1f ly:", cr.Code, cr.Type, cr.Free, cr.At.Loc, routeCost(cr.At, r)))
		for i := 0; i < len(r); {
			k := i
			var acts []string
			for ; k < len(r) && r[k].At.Loc == r[i].At.Loc; k++ {
				s := r[k]
				sp := spares[s.Spare]
				if s.Pickup {
					acts = append(acts, fmt.Sprintf("attach %s (%s for %s)", sp.Code, sp.Type, jobs[s.Job].Dest.Loc))
				} else {
					acts = append(acts, "detach "+sp.Code)
				}
			}
			lines = append(lines, fmt.Sprintf("    %s: %s", r[i].At.Loc, strings.Join(acts, ", ")))
			i = k
		}
	}
	for _, j := range slices.Sorted(maps.Keys(dp.Unassigned)) {
		lines = append(lines, fmt.Sprintf("Unassigned: %s for %s: %s", jobs[j].Type, jobs[j].Dest.Loc, dp.Unassigned[j]))
	}
	return lines
}

const beaconType = "ftl_beacon"

var carrierTypes = []string{"surge_carrier", "surge_platform", "surge_plate"}

// isSpare reports whether d may be taken for a job: not attached or stowed,
// not at an event location or an -OBJ- asteroid, and either an ftl_beacon
// at a belt or another device that's idle.
func isSpare(d model.Device, eventLocs map[string]bool) bool {
	if d.AttachedToDeviceCode != "" || d.StowedInDeviceCode != "" ||
		eventLocs[d.Location] || strings.Contains(d.Location, "-OBJ-") {
		return false
	}
	if d.DeviceType == beaconType {
		return strings.Contains(d.Location, "-BELT-")
	}
	return d.Status == "idle"
}

// deviceJobs lists the devices each need wants: a beacon if it has none,
// then its chosen option's missing devices.
func deviceJobs(needs []need) []job {
	var jobs []job
	for _, n := range sortedNeeds(needs) {
		dest := place{Loc: n.Loc, Pos: n.Pos}
		if !n.Beacon {
			jobs = append(jobs, job{Type: beaconType, Dest: dest})
		}
		for _, d := range n.Devices {
			for range d.Required - d.Current {
				jobs = append(jobs, job{Type: d.DeviceType, Dest: dest})
			}
		}
	}
	return jobs
}
