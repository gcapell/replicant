package main

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/gcapell/replicant/model"
)

// Job is one print, current or queued, with when it should finish.
type Job struct {
	DeviceType string
	Duration   time.Duration // remaining for the current print, print_time for queued ones
	ETA        time.Time     // zero if unknown (the factory isn't printing yet)
}

// Factory is one autofactory's place, state and jobs, in print order.
type Factory struct {
	Code     string
	Location string
	Status   string
	Waiting  string // what it's waiting for, e.g. "C 10/50", if anything
	Jobs     []Job
}

// schedule works out an autofactory's jobs: what it's printing now (if
// anything), then each queued item in order, each starting when the one before
// finishes. printTime gives each device type's print time, from the
// blueprints. A queued type with no blueprint is an error.
func schedule(d *model.Device, printTime map[string]time.Duration, now time.Time) (Factory, error) {
	f := Factory{Code: d.DeviceCode, Location: d.Location, Status: d.Status, Waiting: waiting(d.WaitingFor)}
	var end time.Time // when the job before finishes; zero if unknown
	if p := d.Printing; p.DeviceType != "" {
		var err error
		if end, err = completes(p.PendingStatus, now); err != nil {
			return f, fmt.Errorf("%s printing %s: %w", d.DeviceCode, p.DeviceType, err)
		}
		f.Jobs = append(f.Jobs, Job{DeviceType: p.DeviceType, Duration: max(end.Sub(now), 0), ETA: end})
	}
	for _, q := range d.PrintQueue {
		t, ok := printTime[q.DeviceType]
		if !ok {
			return f, fmt.Errorf("%s: no blueprint for queued %q", d.DeviceCode, q.DeviceType)
		}
		j := Job{DeviceType: q.DeviceType, Duration: t}
		if !end.IsZero() {
			end = end.Add(t)
			j.ETA = end
		}
		f.Jobs = append(f.Jobs, j)
	}
	return f, nil
}

// completes is when a pending operation finishes: completes_at, or else now
// plus eta_seconds.
func completes(p model.PendingStatus, now time.Time) (time.Time, error) {
	if p.CompletesAt == "" {
		return now.Add(time.Duration(p.EtaSeconds) * time.Second), nil
	}
	return time.Parse(time.RFC3339, p.CompletesAt)
}

// waiting shows waiting_for as "C 10/50 Si 0/20", in resource order.
func waiting(w map[model.ResourceType]model.WaitingForStat) string {
	var parts []string
	for _, r := range model.Resources {
		if s, ok := w[r]; ok {
			parts = append(parts, fmt.Sprintf("%s %d/%d", r.Abbrev(), s.Have, s.Need))
		}
	}
	for r, s := range w { // anything we don't know, so it isn't hidden
		if !slices.Contains(model.Resources, r) {
			parts = append(parts, fmt.Sprintf("%s %d/%d", r, s.Have, s.Need))
		}
	}
	return strings.Join(parts, " ")
}

// report prints the factories grouped by location, both sorted, e.g.
//
//	OTHONAL-BELT-1
//	   072FB5E6
//	       autofactory (2h ETA 15:37)
//	       cargo_freighter (20m ETA 15:57)
func report(w io.Writer, fs []Factory, now time.Time) {
	fs = slices.Clone(fs)
	slices.SortFunc(fs, func(a, b Factory) int {
		return cmp.Or(cmp.Compare(a.Location, b.Location), cmp.Compare(a.Code, b.Code))
	})
	loc := "\x00"
	for _, f := range fs {
		if f.Location != loc {
			loc = f.Location
			fmt.Fprintln(w, cmp.Or(loc, "(no location)"))
		}
		head := "   " + f.Code
		if f.Status != "idle" && !strings.HasPrefix(f.Status, "printing") {
			head += " " + f.Status
		}
		if f.Waiting != "" {
			head += " waiting for " + f.Waiting
		}
		fmt.Fprintln(w, head)
		if len(f.Jobs) == 0 {
			fmt.Fprintln(w, "       (nothing queued)")
		}
		for _, j := range f.Jobs {
			if j.ETA.IsZero() {
				fmt.Fprintf(w, "       %s (%s)\n", j.DeviceType, dur(j.Duration))
			} else {
				fmt.Fprintf(w, "       %s (%s ETA %s)\n", j.DeviceType, dur(j.Duration), clock(j.ETA, now))
			}
		}
	}
}

// dur shows a duration briefly: "2h", "1h30m", "20m", "45s".
func dur(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Round(time.Second)/time.Second))
	}
	d = d.Round(time.Minute)
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

// clock shows t in local time as "15:37", with the weekday if it isn't today.
func clock(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	if y, m, d := t.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return t.Format("15:04")
	}
	return t.Format("Mon 15:04")
}
