package main

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// candidate is a star we could prospect from.
type candidate struct {
	Name string
	Pos  model.Position
	Dest string         // where to take the observatory: the system's entry point
	Dist float64        // from the centre
	Dir  model.Position // prospect direction: straight out from the centre
}

// relayingStars returns the stars holding one of relays that is relaying (and
// not stowed).
func relayingStars(relays []model.Device) map[string]bool {
	set := map[string]bool{}
	for _, r := range relays {
		if r.Status == "relaying" && r.StowedInDeviceCode == "" && r.Location != "" {
			set[auto.StarOf(r.Location)] = true
		}
	}
	return set
}

// candidates returns the stars holding a relaying ftl_relay, nearest the centre
// first, along with the centre (their mean position). Each candidate's Dest is
// the star's entry point, not the relay's location: a relay can sit elsewhere
// in the system. Relay stars not in stars are returned in offMap, and those
// whose entry point isn't known in noEntry, both sorted.
func candidates(stars []model.Star, relays []model.Device) (cands []candidate, centre model.Position, offMap, noEntry []string) {
	relaying := relayingStars(relays)
	seen := map[string]bool{}
	for _, s := range stars {
		if !relaying[s.Designation] || seen[s.Designation] {
			continue
		}
		seen[s.Designation] = true
		if s.EntryPoint == "" {
			noEntry = append(noEntry, s.Designation)
			continue
		}
		cands = append(cands, candidate{Name: s.Designation, Pos: s.Position, Dest: s.EntryPoint})
	}
	for name := range relaying {
		if !seen[name] {
			offMap = append(offMap, name)
		}
	}
	slices.Sort(offMap)
	slices.Sort(noEntry)
	if len(cands) == 0 {
		return nil, centre, offMap, noEntry
	}
	for _, c := range cands {
		centre.X += c.Pos.X
		centre.Y += c.Pos.Y
		centre.Z += c.Pos.Z
	}
	n := float64(len(cands))
	centre = model.Position{X: centre.X / n, Y: centre.Y / n, Z: centre.Z / n}
	for i := range cands {
		cands[i].Dist = cands[i].Pos.Distance(centre)
		cands[i].Dir = cands[i].Pos.Delta(centre)
	}
	slices.SortStableFunc(cands, func(a, b candidate) int {
		return cmp.Or(cmp.Compare(a.Dist, b.Dist), cmp.Compare(a.Name, b.Name))
	})
	return cands, centre, offMap, noEntry
}

// obsPlan is what one observatory should do next.
type obsPlan struct {
	Obs       model.Device
	State     string
	Target    *candidate // star it's going to, or prospecting from; nil if neither
	Transport string     // transport it uses, if any
	Steps     []step     // next steps, in order; the last may be a long one
	Problem   string     // why it can't go on, if it can't
}

// Observatory status values.
const (
	compacted   = "compacted"
	compacting  = "compacting"
	unfurling   = "unfurling"
	prospecting = "prospecting"
	idle        = "idle" // assumed: unfurled and not doing anything
)

// planner holds what the plan is made from, and the claims made so far.
type planner struct {
	cands   []candidate
	byName  map[string]*candidate
	devices map[string]model.Device // all devices by code (for transports)
	led     ledger
	claimed map[string]string // star -> observatory going to (or staying at) it
	used    map[string]bool   // transports already taken this run
}

// planAll plans the next steps for each observatory (in device-code order).
// devices must include the transports.
func planAll(obs []model.Device, devices []model.Device, cands []candidate, led ledger) []obsPlan {
	p := &planner{
		cands:   cands,
		byName:  map[string]*candidate{},
		devices: map[string]model.Device{},
		led:     led,
		claimed: map[string]string{},
		used:    map[string]bool{},
	}
	for i := range cands {
		p.byName[cands[i].Name] = &cands[i]
	}
	for _, d := range devices {
		p.devices[d.DeviceCode] = d
	}
	obs = slices.SortedFunc(slices.Values(obs), func(a, b model.Device) int {
		return cmp.Compare(a.DeviceCode, b.DeviceCode)
	})

	// First, claim the stars observatories are already at or heading to, and
	// the transports they're on, so nobody else is planned there.
	for _, o := range obs {
		if t, ok := p.devices[o.AttachedToDeviceCode]; ok {
			p.used[t.DeviceCode] = true
			if t.Status == "travelling" {
				p.claim(auto.StarOf(t.Travel.FinalDestination), o.DeviceCode)
				continue
			}
		}
		if p.usable(auto.StarOf(o.Location)) {
			p.claim(auto.StarOf(o.Location), o.DeviceCode)
		}
	}

	plans := make([]obsPlan, len(obs))
	for i, o := range obs {
		plans[i] = p.plan(o)
	}
	return plans
}

// usable reports whether we could prospect from star (ignoring claims).
func (p *planner) usable(star string) bool {
	return p.byName[star] != nil && !p.led.done(star)
}

func (p *planner) claim(star, obs string) {
	if _, ok := p.claimed[star]; !ok {
		p.claimed[star] = obs
	}
}

// here returns the star o is at, if it should prospect from there.
func (p *planner) here(o model.Device) *candidate {
	star := auto.StarOf(o.Location)
	if p.usable(star) && p.claimed[star] == o.DeviceCode {
		return p.byName[star]
	}
	return nil
}

// next claims and returns the nearest-the-centre star nobody's using, or nil.
func (p *planner) next(o model.Device) *candidate {
	for i := range p.cands {
		c := &p.cands[i]
		if _, taken := p.claimed[c.Name]; !taken && !p.led.done(c.Name) {
			p.claimed[c.Name] = o.DeviceCode
			return c
		}
	}
	return nil
}

func (p *planner) plan(o model.Device) obsPlan {
	pl := obsPlan{Obs: o, State: o.Status}
	t, attached := p.devices[o.AttachedToDeviceCode]
	switch {
	case o.Status == compacting || o.Status == unfurling || o.Status == prospecting:
		pl.State = o.Status + " (long wait)"
		if c := p.byName[auto.StarOf(o.Location)]; c != nil && o.Status != compacting {
			pl.Target = c
		}

	case o.Status == compacted && attached && t.Status == "travelling":
		pl.State = "compacted, on " + t.DeviceCode + " travelling to " + t.Travel.FinalDestination
		pl.Transport = t.DeviceCode
		pl.Target = p.byName[auto.StarOf(t.Travel.FinalDestination)]
		pl.Steps = []step{
			{Kind: waitTrip, Transport: t.DeviceCode},
			{Kind: detach, Transport: t.DeviceCode, Dest: t.Travel.FinalDestination},
			{Kind: unfurl},
		}

	case o.Status == compacted:
		if attached {
			pl.State = "compacted, attached to " + t.DeviceCode
			pl.Transport = t.DeviceCode
		} else if o.AttachedToDeviceCode != "" {
			pl.Problem = "attached to unknown device " + o.AttachedToDeviceCode
			return pl
		}
		if c := p.here(o); c != nil {
			pl.Target = c
			if attached {
				pl.Steps = append(pl.Steps, step{Kind: detach, Transport: t.DeviceCode, Dest: o.Location})
			}
			pl.Steps = append(pl.Steps, step{Kind: unfurl})
			return pl
		}
		if !attached {
			tr, ok := p.transport(o.Location)
			if !ok {
				pl.Problem = "no free surge+attach transport at " + o.Location
				return pl
			}
			pl.Transport = tr.DeviceCode
			pl.Steps = append(pl.Steps, step{Kind: attach, Transport: tr.DeviceCode, Capacity: tr.AttachCapacity})
		}
		c := p.next(o)
		if c == nil {
			pl.Problem = "no star left to prospect from"
			pl.Steps = nil
			return pl
		}
		pl.Target = c
		pl.Steps = append(pl.Steps,
			step{Kind: travel, Transport: pl.Transport, From: o.Location, Dest: c.Dest},
			step{Kind: detach, Transport: pl.Transport, Dest: c.Dest},
			step{Kind: unfurl})

	case o.Status == idle:
		pl.State = "idle (unfurled)"
		if c := p.here(o); c != nil {
			pl.Target = c
			pl.Steps = []step{{Kind: prospect, Star: c.Name, Dir: c.Dir}}
		} else {
			pl.Steps = []step{{Kind: compact}}
		}

	default:
		pl.Problem = fmt.Sprintf("unknown status %q", o.Status)
	}
	return pl
}

// transport claims and returns the free surge+attach transport at loc with the
// smallest attach capacity.
func (p *planner) transport(loc string) (model.Device, bool) {
	var best model.Device
	found := false
	for _, d := range p.devices {
		if d.Location != loc || p.used[d.DeviceCode] || d.Status != "idle" ||
			!slices.Contains(d.Features, "surge") || !slices.Contains(d.Features, "attach") ||
			d.AttachUsed >= d.AttachCapacity {
			continue
		}
		if !found || cmp.Or(cmp.Compare(d.AttachCapacity, best.AttachCapacity), cmp.Compare(d.DeviceCode, best.DeviceCode)) < 0 {
			best, found = d, true
		}
	}
	if found {
		p.used[best.DeviceCode] = true
	}
	return best, found
}

type stepKind int

const (
	waitTrip stepKind = iota // wait for Transport to finish its trip
	attach                   // attach the observatory to Transport
	travel                   // Transport carries it From -> Dest
	detach                   // detach it from Transport, which should be at Dest's star
	unfurl                   // long
	compact                  // long
	prospect                 // long: from Star, in direction Dir
)

// step is one thing to do to an observatory.
type step struct {
	Kind             stepKind
	Transport        string
	Capacity         int // Transport's attach capacity, for the log
	From, Dest, Star string
	Dir              model.Position
}

func (s step) String() string {
	switch s.Kind {
	case waitTrip:
		return "wait for " + s.Transport + " to arrive"
	case attach:
		return fmt.Sprintf("attach to %s (attach capacity %d)", s.Transport, s.Capacity)
	case travel:
		return fmt.Sprintf("%s travels %s -> %s (~10m)", s.Transport, s.From, s.Dest)
	case detach:
		return "detach from " + s.Transport
	case unfurl:
		return "unfurl (hours)"
	case compact:
		return "compact (hours)"
	case prospect:
		return fmt.Sprintf("prospect from %s direction [%.2f, %.2f, %.2f] (12h)", s.Star, s.Dir.X, s.Dir.Y, s.Dir.Z)
	}
	return fmt.Sprintf("step(%d)", s.Kind)
}

// needsHub reports whether any of plans waits on travel.
func needsHub(plans []obsPlan) bool {
	for _, p := range plans {
		for _, s := range p.Steps {
			if s.Kind == waitTrip || s.Kind == travel {
				return true
			}
		}
	}
	return false
}
