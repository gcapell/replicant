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

type amounts = map[model.ResourceType]int

// need is what one event still wants delivered (from its first option).
type need struct {
	Loc     string
	Star    string
	Pos     model.Position
	Want    amounts                // required - current, only where positive
	Devices []model.RequiredDevice // unmet device requirements (not handled yet)
	Met     bool                   // the option is already met
	Beacon  bool                   // an ftl_beacon is at Loc
	Relay   bool                   // Star has a relaying ftl_relay
}

// pickup is one collection: Qty of Res from Loc.
type pickup struct {
	Loc string
	Res model.ResourceType
	Qty int
}

// assignment says where an event's resources will come from. Star is empty
// if no single star can supply everything; Nearest then gives, for each
// resource, the nearest star that could supply that resource on its own
// (empty if none can).
type assignment struct {
	Need    need
	Star    string
	Dist    float64
	Collect []pickup
	Nearest map[model.ResourceType]string
}

// toNeed turns an event into a need, using only its first option.
// FIXME: alternative options are ignored.
func toNeed(loc string, ev *model.LocationEvent) need {
	n := need{Loc: loc, Star: auto.StarOf(loc), Want: amounts{}}
	if len(ev.Progress.Options) == 0 {
		return n
	}
	o := ev.Progress.Options[0]
	n.Met = o.Met
	for _, r := range o.Resources {
		if left := r.Required - r.Current; left > 0 {
			n.Want[r.ResourceType] += left
		}
	}
	for _, d := range o.Devices {
		if d.Current < d.Required {
			n.Devices = append(n.Devices, d)
		}
	}
	return n
}

// isSource reports whether resources at loc may be collected: a planet, moon
// or asteroid belt (never an -OBJ- asteroid or anything else).
func isSource(loc string) bool {
	star := auto.StarOf(loc)
	if auto.IsPlanetOrMoon(star, loc) {
		return true
	}
	rest, ok := strings.CutPrefix(loc, star+"-BELT-")
	return ok && rest != "" && !strings.Contains(rest, "-")
}

// plan assigns each need that wants resources to the nearest star whose
// stock covers all of it. Needs are handled greedily: of those still
// waiting, the one with the nearest possible source goes next, and its
// pickups are taken off the stock before the next choice, so two events
// never count on the same resources. stock (location -> amounts) is
// consumed; starPos gives the position of every star in stock.
//
// The result lists the sourced needs in the order chosen, then those with
// no single source. Needs that want no resources are left out.
func plan(needs []need, stock map[string]amounts, starPos map[string]model.Position) []assignment {
	var pending []need
	for _, n := range needs {
		if len(n.Want) > 0 && !n.Met {
			pending = append(pending, n)
		}
	}
	slices.SortFunc(pending, func(a, b need) int { return cmp.Compare(a.Loc, b.Loc) })

	var out []assignment
	for len(pending) > 0 {
		totals := starTotals(stock)
		best, bestStar, bestDist := -1, "", math.Inf(1)
		for i, n := range pending {
			if s, d := nearestCovering(n.Pos, n.Want, totals, starPos); s != "" && d < bestDist {
				best, bestStar, bestDist = i, s, d
			}
		}
		if best < 0 {
			break
		}
		n := pending[best]
		pending = slices.Delete(pending, best, best+1)
		out = append(out, assignment{Need: n, Star: bestStar, Dist: bestDist, Collect: take(stock, bestStar, n.Want)})
	}

	totals := starTotals(stock)
	for _, n := range pending {
		a := assignment{Need: n, Nearest: map[model.ResourceType]string{}}
		for r, q := range n.Want {
			a.Nearest[r], _ = nearestCovering(n.Pos, amounts{r: q}, totals, starPos)
		}
		out = append(out, a)
	}
	return out
}

// starTotals sums stock per star.
func starTotals(stock map[string]amounts) map[string]amounts {
	t := map[string]amounts{}
	for loc, am := range stock {
		s := auto.StarOf(loc)
		if t[s] == nil {
			t[s] = amounts{}
		}
		for r, q := range am {
			t[s][r] += q
		}
	}
	return t
}

// nearestCovering returns the nearest star (and its distance from pos) whose
// totals cover want, or "" if none does. Ties go to the star name.
func nearestCovering(pos model.Position, want amounts, totals map[string]amounts, starPos map[string]model.Position) (string, float64) {
	best, bestDist := "", math.Inf(1)
	for _, s := range slices.Sorted(maps.Keys(totals)) {
		p, ok := starPos[s]
		if !ok || !covers(totals[s], want) {
			continue
		}
		if d := pos.Distance(p); d < bestDist {
			best, bestDist = s, d
		}
	}
	return best, bestDist
}

func covers(have, want amounts) bool {
	for r, q := range want {
		if have[r] < q {
			return false
		}
	}
	return true
}

// take removes want from the stock at star's locations, largest stockpile
// first, and returns the pickups. The star must cover want.
func take(stock map[string]amounts, star string, want amounts) []pickup {
	var locs []string
	for loc := range stock {
		if auto.StarOf(loc) == star {
			locs = append(locs, loc)
		}
	}
	var out []pickup
	for _, r := range slices.Sorted(maps.Keys(want)) {
		slices.SortFunc(locs, func(a, b string) int {
			return cmp.Or(cmp.Compare(stock[b][r], stock[a][r]), cmp.Compare(a, b))
		})
		left := want[r]
		for _, loc := range locs {
			if left == 0 {
				break
			}
			q := min(left, stock[loc][r])
			if q == 0 {
				continue
			}
			stock[loc][r] -= q
			left -= q
			out = append(out, pickup{Loc: loc, Res: r, Qty: q})
		}
	}
	return out
}

// describe renders the plan, with each sourced assignment's freighter (ds,
// keyed by index into as), then the needs that are already met.
func describe(as []assignment, ds map[int]dispatch, needs []need) []string {
	var lines []string
	for i, a := range as {
		line := fmt.Sprintf("%s requires %s", a.Need.Loc, formatAmounts(a.Need.Want))
		if a.Star != "" {
			line += fmt.Sprintf(" @%s (%.1f ly)", a.Star, a.Dist)
		} else {
			line += ": no single source"
		}
		lines = append(lines, line+flags(a.Need))
		if a.Star != "" {
			if d, ok := ds[i]; ok {
				lines = append(lines, fmt.Sprintf("Dispatching %s (%.1f ly)", d.Freighter.Code, d.Dist))
			} else {
				lines = append(lines, "No idle freighter")
			}
			lines = append(lines, formatPickups(a.Collect)...)
		} else {
			for _, r := range slices.Sorted(maps.Keys(a.Need.Want)) {
				s := a.Nearest[r]
				if s == "" {
					s = "nowhere"
				}
				lines = append(lines, fmt.Sprintf("    %s: nearest %s", resourceName(r), s))
			}
		}
		lines = append(lines, formatDevices(a.Need.Devices)...)
	}
	for _, n := range sortedNeeds(needs) {
		switch {
		case n.Met:
			lines = append(lines, fmt.Sprintf("%s ready to complete%s", n.Loc, flags(n)))
		case len(n.Want) == 0:
			lines = append(lines, fmt.Sprintf("%s needs no resources%s", n.Loc, flags(n)))
			lines = append(lines, formatDevices(n.Devices)...)
		}
	}
	return lines
}

func sortedNeeds(needs []need) []need {
	s := slices.Clone(needs)
	slices.SortFunc(s, func(a, b need) int { return cmp.Compare(a.Loc, b.Loc) })
	return s
}

func flags(n need) string {
	var f []string
	if !n.Beacon {
		f = append(f, "NO BEACON")
	}
	if !n.Relay {
		f = append(f, "NO RELAY")
	}
	if len(f) == 0 {
		return ""
	}
	return " [" + strings.Join(f, ", ") + "]"
}

// formatPickups gives one line per location: "    collect X-3: 50 Carbon".
func formatPickups(ps []pickup) []string {
	byLoc := map[string]amounts{}
	for _, p := range ps {
		if byLoc[p.Loc] == nil {
			byLoc[p.Loc] = amounts{}
		}
		byLoc[p.Loc][p.Res] += p.Qty
	}
	var lines []string
	for _, loc := range slices.Sorted(maps.Keys(byLoc)) {
		lines = append(lines, fmt.Sprintf("    collect %s: %s", loc, formatAmounts(byLoc[loc])))
	}
	return lines
}

func formatDevices(ds []model.RequiredDevice) []string {
	if len(ds) == 0 {
		return nil
	}
	var items []string
	for _, d := range ds {
		items = append(items, fmt.Sprintf("%s(%d/%d)", d.DeviceType, d.Current, d.Required))
	}
	return []string{"    also needs devices (not handled yet): " + strings.Join(items, ", ")}
}

// formatAmounts gives "50 Carbon, 100 Silicates", in resource order.
func formatAmounts(am amounts) string {
	var items []string
	for _, r := range slices.Sorted(maps.Keys(am)) {
		items = append(items, fmt.Sprintf("%d %s", am[r], resourceName(r)))
	}
	return strings.Join(items, ", ")
}

func resourceName(r model.ResourceType) string {
	s := string(r)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
