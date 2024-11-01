package analysis

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

// openTableConnection connects to SQLite database or creates it
// if it doesn't exist.
func openTableConnection(
	tableName string,
	stateTimeHistories *StateTimeHistories,
) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./test_file.db")
	if err != nil {
		return nil, err
	}
	colDeclarations := ""
	for partition, stateHistory := range stateTimeHistories.StateHistories {
		for i := 0; i < stateHistory.StateWidth; i++ {
			colDeclarations += fmt.Sprintf(`%s%d REAL NOT NULL,
			`, partition, i)
		}
	}
	createTableQuery := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		time REAL NOT NULL,
		%s
	);`, tableName, colDeclarations)
	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatalf("Failed to create table")
		return nil, err
	}
	return db, nil
}

// WriteStateTimeHistoriesToSqlite writes all of the data in the state time
// histories to a SQLite database.
func WriteStateTimeHistoriesToSqlite(
	tableName string,
	stateTimeHistories *StateTimeHistories,
) error {
	db, err := openTableConnection(tableName, stateTimeHistories)
	if err != nil {
		return err
	}
	defer db.Close()

	return nil
}
