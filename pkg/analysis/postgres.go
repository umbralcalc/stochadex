package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "github.com/lib/pq"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// openTableConnection connects to PostgreSQL database or creates it
// if it doesn't exist.
func openTableConnection(config *PostgresDbConfig) (*sql.DB, error) {
	connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable",
		config.User, config.Password, config.Dbname)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	// Generate column declarations based on StateTimeHistories
	colDeclarations := ""
	for partition, rowSize := range config.RowSizeByPartitionName {
		for i := 0; i < rowSize; i++ {
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
	);`, config.TableName, colDeclarations)
	_, err = db.Exec(createTableQuery)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// insertEmptyRow inserts a new empty row and returns the
// ID of the created row.
func insertEmptyRowQuery(db *sql.DB, tableName string) (int64, error) {
	var rowId int64
	err := db.QueryRow(fmt.Sprintf(
		"INSERT INTO %s DEFAULT VALUES RETURNING id",
		tableName,
	)).Scan(&rowId)
	return rowId, err
}

// updateColumnGroup updates a specific group of columns
// for a given row ID.
func updateColumnGroupQuery(
	ctx context.Context,
	db *sql.DB,
	tableName string,
	rowId int64,
	columnValues map[string]interface{},
) error {
	setClauses := ""
	args := []interface{}{rowId}
	i := 2
	for column, value := range columnValues {
		setClauses += fmt.Sprintf("%s = $%d, ", column, i)
		args = append(args, value)
		i++
	}
	setClauses = setClauses[:len(setClauses)-2]
	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $1;",
		tableName, setClauses)
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

// PostgresDbConfig
type PostgresDbConfig struct {
	User                   string
	Password               string
	Dbname                 string
	TableName              string
	RowSizeByPartitionName map[string]int
}

// PostgresDbOutputFunction
type PostgresDbOutputFunction struct {
	db *sql.DB
}

func (p *PostgresDbOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {

}

// NewPostgresDbOutputFunction creates a new PostgresDbOutputFunction.
func NewPostgresDbOutputFunction(
	config *PostgresDbConfig,
) *PostgresDbOutputFunction {
	db, err := openTableConnection(config)
	if err != nil {
		panic(err)
	}
	return &PostgresDbOutputFunction{db: db}
}

// asyncWriteColumnGroups performs concurrent column group updates
func asyncWriteColumnGroups(db *sql.DB, tableName string, columnGroups []map[string]interface{}) error {
	// Insert an empty row and get the new row ID
	rowId, err := insertEmptyRowQuery(db, tableName)
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
			if err := updateColumnGroupQuery(ctx, db, tableName, rowId, colGroup); err != nil {
				log.Printf("failed to update column group: %v", err)
			}
		}(columnGroup)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	return nil
}

// WriteStateTimeStorageToPostgres writes all of the data
// in the state time storage to a PostgreSQL database.
func WriteStateTimeHistoriesToPostgres(
	config *PostgresDbConfig,
	storage *simulator.StateTimeStorage,
) {
	// outputFunction := NewPostgresDbOutputFunction(config)
	_ = NewPostgresDbOutputFunction(config)
}
