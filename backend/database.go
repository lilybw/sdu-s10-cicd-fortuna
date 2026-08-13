package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

var sqliteDB *sql.DB

func openDatabase() {
	var err error

	sqliteDB, err = sql.Open("sqlite", "fortunes.db")
	if err != nil {
		fmt.Println("Failed to open database:", err)
		return
	}

	_, err = sqliteDB.Exec(`
		CREATE TABLE IF NOT EXISTS fortunes (
			id TEXT PRIMARY KEY,
			message TEXT NOT NULL
		)
	`)
	if err != nil {
		fmt.Println("Failed to create fortunes table:", err)
		return
	}
	seedDatabase()

	fmt.Println("SQLite database ready")

	loadFortunes()
}

func seedDatabase() {
	var count int

	err := sqliteDB.QueryRow(
		"SELECT COUNT(*) FROM fortunes",
	).Scan(&count)

	if err != nil {
		fmt.Println("Failed to check fortunes:", err)
		return
	}

	if count > 0 {
		return
	}

	seedSQL, err := os.ReadFile("seed.sql")
	if err != nil {
		fmt.Println("Failed to read seed.sql:", err)
		return
	}

	_, err = sqliteDB.Exec(string(seedSQL))
	if err != nil {
		fmt.Println("Failed to seed database:", err)
		return
	}

	fmt.Println("Database seeded")
}

func loadFortunes() {
	rows, err := sqliteDB.Query("SELECT id, message FROM fortunes")
	if err != nil {
		fmt.Println("Failed to load fortunes:", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var f fortune

		err := rows.Scan(&f.ID, &f.Message)
		if err != nil {
			fmt.Println("Failed to read fortune:", err)
			continue
		}

		datastoreDefault.Lock()
		datastoreDefault.m[f.ID] = f
		datastoreDefault.Unlock()
	}
}

func saveFortune(f fortune) error {
	_, err := sqliteDB.Exec(
		"INSERT OR REPLACE INTO fortunes (id, message) VALUES (?, ?)",
		f.ID,
		f.Message,
	)

	return err
}
