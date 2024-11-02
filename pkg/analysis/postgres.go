package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "github.com/lib/pq"
)

// openTableConnection connects to PostgreSQL database or creates it
// if it doesn't exist.
func openTableConnection(
	user string,
	password string,
	dbname string,
	tableName string,
	stateTimeHistories *StateTimeHistories,
) (*sql.DB, error) {
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable",
		user, password, dbname)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Generate column declarations based on StateTimeHistories
	colDeclarations := ""
	for partition, stateHistory := range stateTimeHistories.StateHistories {
		for i := 0; i < stateHistory.StateWidth; i++ {
			colDeclarations += fmt.Sprintf("%s%d DOUBLE PRECISION NOT NULL, ",
				partition, i)
		}
	}

	// Write and execute the generated query string
	createTableQuery := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id SERIAL PRIMARY KEY,
		time DOUBLE PRECISION NOT NULL,
		%s
	);`, tableName, colDeclarations)
	_, err = db.Exec(createTableQuery)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// insertEmptyRow inserts a new empty row and returns the
// ID of the created row.
func insertEmptyRow(db *sql.DB, tableName string) (int64, error) {
	var id int64
	err := db.QueryRow(fmt.Sprintf(
		"INSERT INTO %s DEFAULT VALUES RETURNING id",
		tableName,
	)).Scan(&id)
	return id, err
}

// updateColumnGroup updates a specific group of columns
// for a given row ID.
func updateColumnGroup(
	ctx context.Context,
	db *sql.DB,
	tableName string,
	id int64,
	columnValues map[string]interface{},
) error {
	// Build column update statements
	setClauses := ""
	args := []interface{}{id}
	i := 2
	for column, value := range columnValues {
		setClauses += fmt.Sprintf("%s = $%d, ", column, i)
		args = append(args, value)
		i++
	}

	// Remove the trailing comma and space
	setClauses = setClauses[:len(setClauses)-2]

	// Build and execute the SQL UPDATE statement
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $1;",
		tableName, setClauses)
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

// asyncWriteColumnGroups performs concurrent column group updates
func asyncWriteColumnGroups(db *sql.DB, tableName string, columnGroups []map[string]interface{}) error {
	// Insert an empty row and get the new row ID
	rowID, err := insertEmptyRow(db, tableName)
	if err != nil {
		return fmt.Errorf("failed to insert empty row: %v", err)
	}

	// Create a WaitGroup to synchronize goroutines
	var wg sync.WaitGroup
	ctx := context.Background()

	// Iterate over each column group and launch a goroutine for each
	for _, columnGroup := range columnGroups {
		wg.Add(1)
		go func(colGroup map[string]interface{}) {
			defer wg.Done()
			if err := updateColumnGroup(ctx, db, tableName, rowID, colGroup); err != nil {
				log.Printf("failed to update column group: %v", err)
			}
		}(columnGroup)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	return nil
}

// WriteStateTimeHistoriesToPostgres writes all of the data
// in the state time histories to a PostgreSQL database.
func WriteStateTimeHistoriesToPostgres(
	user string,
	password string,
	dbname string,
	tableName string,
	stateTimeHistories *StateTimeHistories,
) error {
	db, err := openTableConnection(
		user,
		password,
		dbname,
		tableName,
		stateTimeHistories,
	)
	if err != nil {
		return err
	}
	defer db.Close()
	_ = []float64{
		stateTimeHistories.TimestepsHistory.Values.AtVec(
			stateTimeHistories.TimestepsHistory.StateHistoryDepth - 1,
		),
	}

	return nil
}
