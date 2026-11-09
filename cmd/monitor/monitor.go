// monitor keeps the store (store/schema.sql) up to date. It is the only
// program that writes to it.
//
// It connects to the event stream first, resuming after the last event
// recorded (or after -since ID), so nothing that happens during startup is
// missed. Events wait in a queue while it does its startup work:
//   - if the store has no devices it fetches them all;
//   - if it has no stars it fetches the stars nearest a replicant in a
//     heaven_vessel, then reads /locations/STAR for each, storing the star's
//     details, planets and belts. A system we can't read ("No replicant in
//     system", apparently meaning we haven't scanned it) is skipped. It's all
//     one transaction, so if anything fails the stars table stays empty and
//     the next run starts again;
//   - every time, it replaces the inventory table with /inventory (planets,
//     moons and belts), and the blueprints table with /blueprints.
//
// With -catchup EVENT,... it then applies every recorded event of those
// types again, oldest first (see catchUp).
//
// It then files a survey_system task for each relay star not fully surveyed
// and gives the free survey teams one each (survey.go). -n just prints that
// plan, from the store as it is, and exits. After that it calls the handler
// for every assigned task, highest priority first (handlers.go).
//
// Then, until interrupted, it records each event in the events table,
// applies the ones it handles (apply.go) and prints it.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/model"
	"github.com/gcapell/replicant/store"
)

func main() {
	path := flag.String("db", store.DefaultPath(), "database file")
	nStars := flag.Int("stars", 60, "how many of the nearest stars to fetch")
	since := flag.String("since", "", "replay events after this event ID (default: after the last one recorded)")
	catchup := flag.String("catchup", "", "on startup, apply every recorded event of these comma-separated types again, oldest first (e.g. device.deployed,relay.activated)")
	dryRun := flag.Bool("n", false, "print the survey tasks startup would file and assign, from the store as it is, and exit (no API calls, no writes)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *dryRun {
		if err := planOnly(ctx, *path); err != nil {
			log.Fatal(err)
		}
		return
	}
	db, err := store.Open(ctx, *path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	start, replaying := *since, *since != ""
	if !replaying {
		if start, err = store.LastEventID(ctx, db); err != nil {
			log.Fatal(err)
		}
	}
	events := make(chan received, 10000)
	go receive(ctx, api.SSEStream, start, events)

	if err := snapshot(ctx, db, *nStars); err != nil {
		log.Fatal(err)
	}
	if err := readInventory(ctx, db); err != nil {
		log.Fatal(err)
	}
	if err := readBlueprints(ctx, db); err != nil {
		log.Fatal(err)
	}
	p := &printer{w: os.Stdout}
	if *catchup != "" {
		if err := catchUp(ctx, db, strings.Split(*catchup, ","), p); err != nil {
			log.Fatal(err)
		}
	}
	if err := fileSurveys(ctx, db, os.Stdout); err != nil {
		log.Fatal(err)
	}
	if err := runAssigned(ctx, db); err != nil {
		log.Fatal(err)
	}
	log.Print("startup done")

	err = recordAll(ctx, db, events, p, replaying)
	if ctx.Err() == nil {
		log.Fatal(err)
	}
}

// planOnly prints the survey plan from the store, read-only.
func planOnly(ctx context.Context, path string) error {
	db, err := store.OpenReadOnly(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	p, err := readSurveyPlan(ctx, db)
	if err != nil {
		return err
	}
	p.print(os.Stdout)
	return nil
}

// snapshot fills each empty table from the API.
func snapshot(ctx context.Context, db *sql.DB, nStars int) error {
	if err := ifEmpty(ctx, db, "devices", fetchDevices); err != nil {
		return err
	}
	return ifEmpty(ctx, db, "stars", func(ctx context.Context, tx *sql.Tx) error {
		return fetchStars(ctx, tx, nStars)
	})
}

// ifEmpty runs fill in a transaction if table has no rows.
func ifEmpty(ctx context.Context, db *sql.DB, table string, fill func(context.Context, *sql.Tx) error) error {
	n, err := store.Count(ctx, db, table)
	if err != nil {
		return err
	}
	if n > 0 {
		log.Printf("%s: %d rows already", table, n)
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fill(ctx, tx); err != nil {
		return err
	}
	if n, err = store.Count(ctx, tx, table); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("%s: fetched %d", table, n)
	return nil
}

func fetchDevices(ctx context.Context, tx *sql.Tx) error {
	for page, err := range api.Paged[model.DeviceList](ctx, "/devices?limit=50") {
		if err != nil {
			return err
		}
		for _, d := range page.Devices {
			if err := store.PutDevice(ctx, tx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func fetchStars(ctx context.Context, tx *sql.Tx, n int) error {
	replicant, err := store.ReplicantInVessel(ctx, tx)
	if err != nil {
		return err
	}
	if replicant == "" {
		return errors.New("no heaven_vessel carrying a replicant_matrix, so no replicant to find stars near")
	}
	stars, err := api.Stars(ctx, replicant, n)
	if err != nil {
		return err
	}
	if len(stars.Stars) == 0 {
		return errors.New("no stars returned near replicant " + replicant)
	}
	log.Printf("%d stars nearest replicant %s", len(stars.Stars), replicant)
	for _, s := range stars.Stars {
		if err := store.PutStar(ctx, tx, s); err != nil {
			return err
		}
	}
	return readSystems(ctx, tx, stars.Stars)
}

// readSystems stores /locations/STAR for each star. Any error other than
// "No replicant in system" stops it.
func readSystems(ctx context.Context, tx *sql.Tx, stars []model.Star) error {
	read, skipped := 0, 0
	for _, s := range stars {
		star := s.Designation
		loc, err := api.Location(ctx, star)
		if noPresence(err) {
			skipped++
			continue
		}
		if err != nil {
			return err
		}
		if loc.Location != star {
			return fmt.Errorf("asked for /locations/%s, got %q", star, loc.Location)
		}
		if err := store.PutSystem(ctx, tx, *loc); err != nil {
			return err
		}
		read++
	}
	log.Printf("systems: read %d, skipped %d we can't read yet", read, skipped)
	return nil
}

// readInventory replaces the inventory table with /inventory.
func readInventory(ctx context.Context, db *sql.DB) error {
	all, err := api.InventoryAt(ctx, "")
	if err != nil {
		return err
	}
	if err := inTx(ctx, db, func(tx *sql.Tx) error { return store.ReplaceInventory(ctx, tx, all) }); err != nil {
		return err
	}
	log.Printf("inventory: %d locations hold resources", len(all))
	return nil
}

// readBlueprints replaces the blueprints table with /blueprints.
func readBlueprints(ctx context.Context, db *sql.DB) error {
	bs, err := api.AllBlueprints(ctx)
	if err != nil {
		return err
	}
	if err := inTx(ctx, db, func(tx *sql.Tx) error { return store.ReplaceBlueprints(ctx, tx, bs.Blueprints) }); err != nil {
		return err
	}
	log.Printf("blueprints: %d device types", len(bs.Blueprints))
	return nil
}

// noPresence reports whether err is /locations refusing because we have no
// replicant in the system.
func noPresence(err error) bool {
	var he *api.HTTPError
	return errors.As(err, &he) && strings.Contains(string(he.Body), "No replicant in system")
}

func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
