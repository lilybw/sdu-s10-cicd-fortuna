package main

// ---------------------------------------------------------------------
// Tests for database.go
//
// How to run:
//   cd backend
//   go test -v ./...
//
// What's going on here (quick overview):
//
// database.go keeps its state in two PACKAGE-LEVEL variables:
//   - sqliteDB         (the *sql.DB connection)
//   - datastoreDefault (the in-memory map, defined in main.go)
//
// Because those are global, every test that touches the database
// could accidentally leak state into the next test (e.g. leftover
// rows, or a file called fortunes.db sitting in the wrong folder).
// To keep tests isolated we do two things before each test:
//
//   1. testTempDir(t)   -> moves the test into a fresh temp folder,
//                          so "fortunes.db" and "seed.sql" never touch
//                          your real project files, and each test gets
//                          its own empty database file.
//   2. resetGlobals(t)  -> closes any open DB connection and empties
//                          datastoreDefault, so tests don't see data
//                          left behind by a previous test.
//
// Go runs tests in one file sequentially by default (no t.Parallel()
// calls here), so this reset pattern is safe.
// ---------------------------------------------------------------------

import (
	"database/sql"
	"os"
	"testing"
)

// testTempDir switches the current working directory to a throwaway
// temp folder for the duration of the test, and switches back
// automatically when the test finishes (t.Cleanup handles that).
func testTempDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir() // Go deletes this folder for us after the test

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not read working directory: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("could not switch to temp directory: %v", err)
	}

	t.Cleanup(func() {
		os.Chdir(original)
	})
}

// resetGlobals closes any existing DB connection and clears the
// in-memory fortune map, so each test starts from a clean slate.
func resetGlobals(t *testing.T) {
	t.Helper()

	if sqliteDB != nil {
		sqliteDB.Close()
		sqliteDB = nil
	}

	datastoreDefault.Lock()
	datastoreDefault.m = map[string]fortune{}
	datastoreDefault.Unlock()

	t.Cleanup(func() {
		if sqliteDB != nil {
			sqliteDB.Close()
			sqliteDB = nil
		}
	})
}

// openBareDB opens the SQLite file and creates the fortunes table,
// WITHOUT seeding or loading anything. This mirrors the first half of
// openDatabase(), so tests that only care about seedDatabase() or
// loadFortunes() can start from an empty-but-ready table.
func openBareDB(t *testing.T) {
	t.Helper()

	var err error
	sqliteDB, err = sql.Open("sqlite", "fortunes.db")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	_, err = sqliteDB.Exec(`
		CREATE TABLE IF NOT EXISTS fortunes (
			id TEXT PRIMARY KEY,
			message TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create fortunes table: %v", err)
	}
}

// ---------------------------------------------------------------------
// openDatabase()
// ---------------------------------------------------------------------

func TestOpenDatabase_CreatesFileAndTable(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)

	openDatabase()

	if sqliteDB == nil {
		t.Fatal("expected sqliteDB to be set after openDatabase(), got nil")
	}

	if _, err := os.Stat("fortunes.db"); err != nil {
		t.Fatalf("expected fortunes.db to exist on disk, got error: %v", err)
	}

	// Querying the table should not error, which proves it was created.
	var count int
	err := sqliteDB.QueryRow("SELECT COUNT(*) FROM fortunes").Scan(&count)
	if err != nil {
		t.Fatalf("expected fortunes table to exist, query failed: %v", err)
	}
}

func TestOpenDatabase_LoadsExistingRowsIntoMemory(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)

	// First call creates the DB (table starts empty, no seed.sql present
	// so seeding is skipped -> table stays empty, which is fine here).
	openDatabase()

	// Manually insert a row directly, simulating "data that was already
	// in the database from a previous run".
	if err := saveFortune(fortune{ID: "42", Message: "reused across restarts"}); err != nil {
		t.Fatalf("setup: failed to insert row: %v", err)
	}

	// Simulate the app restarting: close the connection and clear memory,
	// then open the database again.
	sqliteDB.Close()
	sqliteDB = nil
	datastoreDefault.Lock()
	datastoreDefault.m = map[string]fortune{}
	datastoreDefault.Unlock()

	openDatabase()

	datastoreDefault.RLock()
	got, ok := datastoreDefault.m["42"]
	datastoreDefault.RUnlock()

	if !ok {
		t.Fatal("expected fortune 42 to be loaded into memory after reopening the database")
	}
	if got.Message != "reused across restarts" {
		t.Errorf("got message %q, want %q", got.Message, "reused across restarts")
	}
}

// ---------------------------------------------------------------------
// seedDatabase()
// ---------------------------------------------------------------------

func TestSeedDatabase_SeedsWhenTableIsEmpty(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	seedSQL := `INSERT INTO fortunes (id, message) VALUES ('1', 'seeded fortune one');`
	if err := os.WriteFile("seed.sql", []byte(seedSQL), 0644); err != nil {
		t.Fatalf("setup: failed to write seed.sql: %v", err)
	}

	seedDatabase()

	var count int
	if err := sqliteDB.QueryRow("SELECT COUNT(*) FROM fortunes").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d rows after seeding, want 1", count)
	}

	var message string
	if err := sqliteDB.QueryRow("SELECT message FROM fortunes WHERE id = '1'").Scan(&message); err != nil {
		t.Fatalf("failed to read seeded row: %v", err)
	}
	if message != "seeded fortune one" {
		t.Errorf("got message %q, want %q", message, "seeded fortune one")
	}
}

func TestSeedDatabase_SkipsWhenTableAlreadyHasRows(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	// Put one row in the table ourselves, BEFORE calling seedDatabase().
	if err := saveFortune(fortune{ID: "1", Message: "already here"}); err != nil {
		t.Fatalf("setup: failed to insert row: %v", err)
	}

	// Note: no seed.sql file exists in this temp dir at all. If
	// seedDatabase() tried to seed anyway, it would fail to read the
	// file. The fact that this doesn't error proves it skipped seeding.
	seedDatabase()

	var count int
	if err := sqliteDB.QueryRow("SELECT COUNT(*) FROM fortunes").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("got %d rows, want 1 (seeding should have been skipped)", count)
	}
}

func TestSeedDatabase_MissingSeedFileDoesNotPanic(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	// Table is empty AND there is no seed.sql in this temp dir.
	// seedDatabase() should just print a message and return quietly,
	// not crash the program.
	seedDatabase()

	var count int
	if err := sqliteDB.QueryRow("SELECT COUNT(*) FROM fortunes").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("got %d rows, want 0 (nothing should have been inserted)", count)
	}
}

// ---------------------------------------------------------------------
// saveFortune()
// ---------------------------------------------------------------------

func TestSaveFortune_InsertsNewRow(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	err := saveFortune(fortune{ID: "7", Message: "lucky number seven"})
	if err != nil {
		t.Fatalf("saveFortune returned an error: %v", err)
	}

	var message string
	err = sqliteDB.QueryRow("SELECT message FROM fortunes WHERE id = '7'").Scan(&message)
	if err != nil {
		t.Fatalf("failed to read inserted row: %v", err)
	}
	if message != "lucky number seven" {
		t.Errorf("got message %q, want %q", message, "lucky number seven")
	}
}

func TestSaveFortune_ReplacesExistingRowWithSameID(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	if err := saveFortune(fortune{ID: "7", Message: "original message"}); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if err := saveFortune(fortune{ID: "7", Message: "updated message"}); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	var count int
	if err := sqliteDB.QueryRow("SELECT COUNT(*) FROM fortunes WHERE id = '7'").Scan(&count); err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("got %d rows with id 7, want exactly 1 (INSERT OR REPLACE should not duplicate)", count)
	}

	var message string
	if err := sqliteDB.QueryRow("SELECT message FROM fortunes WHERE id = '7'").Scan(&message); err != nil {
		t.Fatalf("failed to read row: %v", err)
	}
	if message != "updated message" {
		t.Errorf("got message %q, want %q", message, "updated message")
	}
}

// ---------------------------------------------------------------------
// loadFortunes()
// ---------------------------------------------------------------------

func TestLoadFortunes_PopulatesInMemoryStore(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	// Insert two rows directly into SQLite using saveFortune, then wipe
	// the in-memory map so we can prove loadFortunes() is what fills it.
	if err := saveFortune(fortune{ID: "1", Message: "first"}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := saveFortune(fortune{ID: "2", Message: "second"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	datastoreDefault.Lock()
	datastoreDefault.m = map[string]fortune{}
	datastoreDefault.Unlock()

	loadFortunes()

	datastoreDefault.RLock()
	defer datastoreDefault.RUnlock()

	if len(datastoreDefault.m) != 2 {
		t.Fatalf("got %d fortunes in memory, want 2", len(datastoreDefault.m))
	}
	if datastoreDefault.m["1"].Message != "first" {
		t.Errorf("got %q for id 1, want %q", datastoreDefault.m["1"].Message, "first")
	}
	if datastoreDefault.m["2"].Message != "second" {
		t.Errorf("got %q for id 2, want %q", datastoreDefault.m["2"].Message, "second")
	}
}

func TestLoadFortunes_EmptyTableLeavesStoreEmpty(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)

	datastoreDefault.Lock()
	datastoreDefault.m = map[string]fortune{}
	datastoreDefault.Unlock()

	loadFortunes()

	datastoreDefault.RLock()
	defer datastoreDefault.RUnlock()

	if len(datastoreDefault.m) != 0 {
		t.Errorf("got %d fortunes in memory, want 0", len(datastoreDefault.m))
	}
}
