// Command print_queue shows what each autofactory is printing and has queued,
// with when each print should finish. See README.md.
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"time"

	"github.com/gcapell/replicant/api"
	"github.com/gcapell/replicant/store"
)

func main() {
	dbPath := flag.String("db", store.DefaultPath(), "store path")
	flag.Parse()
	ctx := context.Background()

	db, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	codes, printTime, err := load(ctx, db)
	db.Close()
	if err != nil {
		log.Fatal(err)
	}

	var fs []Factory
	for _, code := range codes {
		d, err := api.Device(ctx, code)
		if err != nil {
			log.Fatal(err)
		}
		if d.DeviceType != "autofactory" {
			log.Fatalf("%s is a %s, not an autofactory", code, d.DeviceType)
		}
		f, err := schedule(d, printTime, time.Now())
		if err != nil {
			log.Fatal(err)
		}
		fs = append(fs, f)
	}
	report(os.Stdout, fs, time.Now())
}

// load reads the autofactories' codes and every device type's print time from
// the store.
func load(ctx context.Context, db *sql.DB) ([]string, map[string]time.Duration, error) {
	var codes []string
	rows, err := db.QueryContext(ctx, `SELECT device_code FROM devices WHERE device_type = 'autofactory' ORDER BY device_code`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, nil, err
		}
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	printTime := map[string]time.Duration{}
	rows, err = db.QueryContext(ctx, `SELECT device_type, print_time FROM blueprints`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var t string
		var s int
		if err := rows.Scan(&t, &s); err != nil {
			return nil, nil, err
		}
		printTime[t] = time.Duration(s) * time.Second
	}
	return codes, printTime, rows.Err()
}
