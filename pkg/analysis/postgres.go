package analysis

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// PostgresDb is a struct which can be configured to define interactions
// with a PostgresSQL database.
type PostgresDb struct {
	User      string
	Password  string
	Dbname    string
	TableName string
	// DB is the database/sql handle used for reads and writes. Set it to your own handle —
	// sql.Open with any DSN or driver (a remote TimescaleDB or another Postgres-wire database
	// with host/port/sslmode, or a pooled *sql.DB) — to use the clean database/sql write path.
	// Leave it nil and OpenTableConnection opens a local Postgres from User/Password/Dbname.
	DB *sql.DB
}

// NewPostgresDb returns a PostgresDb backed by a caller-provided database/sql handle — the
// clean database/sql write path. The handle may target any Postgres-wire database (Postgres,
// TimescaleDB, QuestDB, …) opened with any DSN or driver; tableName is the destination table.
func NewPostgresDb(db *sql.DB, tableName string) *PostgresDb {
	return &PostgresDb{DB: db, TableName: tableName}
}

// OpenTableConnection connects to the PostgreSQL database or creates it
// if it doesn't exist.
func (p *PostgresDb) OpenTableConnection() error {
	// Open a local Postgres from the credential fields only when the caller has not supplied
	// their own handle. A caller-provided DB (any DSN/driver — remote Timescale, QuestDB, a
	// pool) is used as-is; the CREATE TABLE below still runs so the destination table exists.
	if p.DB == nil {
		connStr := fmt.Sprintf("user=%s password=%s dbname=%s sslmode=disable",
			p.User, p.Password, p.Dbname)
		var err error
		p.DB, err = sql.Open("postgres", connStr)
		if err != nil {
			return err
		}
	}
	createTableQuery := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		partition_name TEXT NOT NULL,
		time DOUBLE PRECISION NOT NULL,
		state FLOAT8[] NOT NULL,
		PRIMARY KEY (partition_name, time)
	);`, p.TableName)
	if _, err := p.DB.Exec(createTableQuery); err != nil {
		return err
	}
	return nil
}

// WriteState writes a new partition state value to the database.
func (p *PostgresDb) WriteState(
	partitionName string,
	time float64,
	state []float64,
) error {
	tx, err := p.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %v", err)
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
        INSERT INTO %s (partition_name, time, state)
        VALUES ($1, $2, $3)
        ON CONFLICT (partition_name, time)
        DO UPDATE SET state = EXCLUDED.state
    `, p.TableName))
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %v", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(partitionName, time, pq.Array(state))
	if err != nil {
		return fmt.Errorf(
			"failed to execute statement for %s: %v",
			partitionName,
			err,
		)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}
	return nil
}

// ReadStateInRange retrieves all entries between a specified
// start and end time range for a given partition.
func (p *PostgresDb) ReadStateInRange(
	partitionName string,
	startTime float64,
	endTime float64,
) (*sql.Rows, error) {
	query := fmt.Sprintf(`
        SELECT time, state
        FROM %s
        WHERE partition_name = $1 AND time BETWEEN $2 AND $3
        ORDER BY time ASC
    `, p.TableName)
	rows, err := p.DB.Query(query, partitionName, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to query data: %v", err)
	}
	return rows, nil
}

// PostgresDbOutputFunction writes the data from the simulation to
// a PostgresSQL database when the simulator.OutputCondition is met.
type PostgresDbOutputFunction struct {
	db *PostgresDb
}

func (p *PostgresDbOutputFunction) Configure(*simulator.Settings) {}

func (p *PostgresDbOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	p.db.WriteState(partitionName, cumulativeTimesteps, state)
}

// NewPostgresDbOutputFunction creates a new PostgresDbOutputFunction.
func NewPostgresDbOutputFunction(
	db *PostgresDb,
) *PostgresDbOutputFunction {
	err := db.OpenTableConnection()
	if err != nil {
		panic(err)
	}
	return &PostgresDbOutputFunction{db: db}
}

// NewStateTimeStorageFromPostgresDb reads from a PostgreSQL database over
// a pre-defined time interval into a simulator.StateTimeStorage struct.
func NewStateTimeStorageFromPostgresDb(
	db *PostgresDb,
	partitionNames []string,
	startTime float64,
	endTime float64,
) (*simulator.StateTimeStorage, error) {
	err := db.OpenTableConnection()
	if err != nil {
		return nil, err
	}
	storage := simulator.NewStateTimeStorage()
	for _, partitionName := range partitionNames {
		rows, err := db.ReadStateInRange(
			partitionName,
			startTime,
			endTime,
		)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var time float64
			var state []float64
			if err := rows.Scan(&time, pq.Array(&state)); err != nil {
				return nil, fmt.Errorf("failed to scan row: %v", err)
			}
			storage.Append(partitionName, time, state)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating rows: %v", err)
		}
	}
	return storage, nil
}

// WriteStateTimeStorageToPostgresDb writes all of the data
// in the state time storage to a PostgreSQL database.
func WriteStateTimeStorageToPostgresDb(
	db *PostgresDb,
	storage *simulator.StateTimeStorage,
) {
	generator := simulator.NewConfigGenerator()
	times := storage.GetTimes()
	outputFunction := NewPostgresDbOutputFunction(db)
	generator.SetSimulation(
		&simulator.SimulationConfig{
			OutputCondition: &simulator.EveryStepOutputCondition{},
			OutputFunction:  outputFunction,
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: len(times) - 1,
			},
			TimestepFunction: &general.FromStorageTimestepFunction{Data: times},
			InitTimeValue:    times[0],
		},
	)
	for _, name := range storage.GetNames() {
		data := storage.GetValues(name)
		generator.SetPartition(
			&simulator.PartitionConfig{
				Name:              name,
				Iteration:         &general.FromStorageIteration{Data: data},
				Params:            simulator.NewParams(make(map[string][]float64)),
				InitStateValues:   data[0],
				StateHistoryDepth: 1,
				Seed:              0,
			},
		)
	}
	coordinator := simulator.NewPartitionCoordinator(generator.GenerateConfigs())
	coordinator.Run()
}
