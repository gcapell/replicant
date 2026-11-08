// migrate_db brings the database into line with store/schema.sql. Every
// command that opens the store already does this, but refuses to drop tables
// or columns; migrate_db -allow-drops lets it.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/gcapell/replicant/store"
)

func main() {
	path := flag.String("db", store.DefaultPath(), "database file")
	allowDrops := flag.Bool("allow-drops", false, "drop tables and columns no longer in the schema, losing their data")
	flag.Parse()

	open := store.Open
	if *allowDrops {
		open = store.OpenAllowingDrops
	}
	db, err := open(context.Background(), *path)
	if err != nil {
		log.Fatal(err)
	}
	db.Close()
	log.Printf("%s matches the schema", *path)
}
