package main

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "app.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS notes (
		id INTEGER PRIMARY KEY,
		body TEXT NOT NULL
	)`)
	if err != nil {
		log.Fatal(err)
	}

	db.Exec(`INSERT INTO notes (body) VALUES (?)`, "hello")

	var body string
	db.QueryRow(`SELECT body FROM notes WHERE id = 1`).Scan(&body)
	log.Println(body)
}
