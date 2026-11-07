package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/auto"
	"github.com/gcapell/replicant/model"
)

// surveyTimeout bounds how long we wait for one system's survey to complete.
const surveyTimeout = 12 * time.Hour

// surveyAll runs every team's route at once. A team that fails stops there;
// the others carry on. It returns all the teams' errors.
func surveyAll(ctx context.Context, hub *api.Hub, routes []route) error {
	var wg sync.WaitGroup
	errs := make([]error, len(routes))
	for i, r := range routes {
		if len(r.Stops) == 0 {
			continue
		}
		wg.Go(func() {
			if err := surveyRoute(ctx, hub, r); err != nil {
				log.Printf("team %s stopped: %v", r.Team.Transport, err)
				errs[i] = fmt.Errorf("team %s: %w", r.Team.Transport, err)
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

func surveyRoute(ctx context.Context, hub *api.Hub, r route) error {
	for i, s := range r.Stops {
		log.Printf("team %s: system %d of %d: %s (%d planets, %d moons to scan)",
			r.Team.Transport, i+1, len(r.Stops), s.Name, s.Planets, s.Moons)
		if err := surveySystem(ctx, hub, r.Team, s); err != nil {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	log.Printf("team %s: route finished", r.Team.Transport)
	return nil
}

// surveySystem takes the team to the system's entry point, lets the drones
// loose to scan it, then gathers them back onto the transport.
func surveySystem(ctx context.Context, hub *api.Hub, t team, s system) error {
	if s.EntryPoint == "" {
		return errors.New("no entry point known")
	}
	if err := auto.GoTo(ctx, hub, t.Transport, s.EntryPoint); err != nil {
		return err
	}

	crew := append([]string{t.Controller}, t.Drones...)
	if err := api.DetachDevice(ctx, t.Transport, crew...); err != nil {
		return fmt.Errorf("detach: %w", err)
	}
	log.Printf("team %s: detached %d devices at %s", t.Transport, len(crew), s.EntryPoint)

	if err := survey(ctx, hub, t.Controller, s.Name); err != nil {
		return err
	}

	log.Printf("team %s: recalling %d devices to %s", t.Transport, len(crew), s.EntryPoint)
	if err := auto.GoToAll(ctx, hub, crew, s.EntryPoint); err != nil {
		return err
	}
	if err := api.AttachDevice(ctx, t.Transport, crew...); err != nil {
		return fmt.Errorf("attach: %w", err)
	}
	log.Printf("team %s: reattached %d devices; %s done", t.Transport, len(crew), s.Name)
	return nil
}

// survey sets the controller's survey_system directive and waits for it to
// complete, logging its digests along the way.
func survey(ctx context.Context, hub *api.Hub, controller, star string) error {
	done := hub.Expect(api.AnyOf(
		api.ForDevice[model.DirectiveCompleted](controller),
		api.ForDevice[model.DirectiveCleared](controller),
	))
	digestCtx, stopDigests := context.WithCancel(ctx)
	defer stopDigests()
	go logDigests(digestCtx, hub, controller, star)

	if err := api.SurveySystem(ctx, controller); err != nil {
		done.Cancel()
		return fmt.Errorf("survey_system: %w", err)
	}
	log.Printf("%s surveying %s", controller, star)

	waitCtx, cancel := context.WithTimeout(ctx, surveyTimeout)
	defer cancel()
	ev, err := done.Wait(waitCtx)
	if err != nil {
		return fmt.Errorf("waiting for survey: %w", err)
	}
	switch p := ev.Payload.(type) {
	case *model.DirectiveCompleted:
		log.Printf("%s completed %s at %s", controller, p.Directive, star)
	case *model.DirectiveCleared:
		return fmt.Errorf("%s's directive %s was cleared before completing", controller, p.PreviousDirective)
	}
	return nil
}

// logDigests logs each ami.survey.digest from controller until ctx is done.
func logDigests(ctx context.Context, hub *api.Hub, controller, star string) {
	for {
		w := hub.Expect(api.ForDevice[model.SurveyDigest](controller))
		ev, err := w.Wait(ctx)
		if err != nil {
			w.Cancel()
			return
		}
		r := ev.Payload.(*model.SurveyDigest).Report
		log.Printf("%s surveying %s: %d/%d scanned (%d remaining); drones busy %d, cruising %d, idle %d",
			controller, star, r.Progress.Scanned, r.Progress.Total, r.Progress.Remaining, r.Busy, r.Cruising, r.Idle)
	}
}
