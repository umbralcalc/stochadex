package api

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"gonum.org/v1/gonum/floats"
)

// The CI Postgres service's credentials (see .github/workflows/ci.yml); host and
// port come from PGHOST / PGPORT, as lib/pq reads them.
const (
	testPostgresUser     = "stochadexuser"
	testPostgresPassword = "stochadexpassword"
	testPostgresDbname   = "stochadexdb"
)

// requirePostgres skips the test unless the CI Postgres is reachable.
func requirePostgres(t *testing.T) {
	t.Helper()
	db, err := sql.Open("postgres", fmt.Sprintf(
		"user=%s password=%s dbname=%s sslmode=disable connect_timeout=2",
		testPostgresUser, testPostgresPassword, testPostgresDbname))
	if err != nil {
		t.Skipf("no Postgres driver: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("no Postgres reachable (runs in CI's postgres service): %v", err)
	}
}

// TestPostgresSinkWritesARun runs a config whose output is the postgres sink
// through the CLI's entry point, reads the table back, and checks it holds
// exactly the run RunToStorage returns. The sink connects only when the run
// starts (it is wrapped to defer the connection), so this is the proof that the
// deferred sink still writes every row correctly to a real database.
func TestPostgresSinkWritesARun(t *testing.T) {
	requirePostgres(t)
	table := fmt.Sprintf("sink_test_%d", time.Now().UnixNano())
	db := &analysis.PostgresDb{
		User: testPostgresUser, Password: testPostgresPassword,
		Dbname: testPostgresDbname, TableName: table,
	}
	t.Cleanup(func() {
		if db.DB != nil {
			db.DB.Exec("DROP TABLE IF EXISTS " + table)
		}
	})

	yaml := fmt.Sprintf(batchConfigYAML, "{type: every_step}", "unused")
	yaml = replaceOnce(t, yaml,
		`output_function: {type: json_log, path: "unused"}`,
		fmt.Sprintf(`output_function: {type: postgres, user: %s, password: %s, dbname: %s, table: %s}`,
			testPostgresUser, testPostgresPassword, testPostgresDbname, table))
	path := filepath.Join(t.TempDir(), "postgres.yaml")
	writeFile(t, path, yaml)

	var err error
	captureStdout(t, func() { err = Execute([]string{"stochadex", "--config", path}) })
	if err != nil {
		t.Fatalf("running with a postgres sink: %v", err)
	}

	reference, err := RunToStorage(LoadApiRunConfigFromYaml(path))
	if err != nil {
		t.Fatal(err)
	}
	fromDb, err := analysis.NewStateTimeStorageFromPostgresDb(
		db, []string{"first", "second"}, 0.0, 30.0)
	if err != nil {
		t.Fatalf("reading the run back: %v", err)
	}
	for _, name := range []string{"first", "second"} {
		want, got := reference.Storage.GetValues(name), fromDb.GetValues(name)
		if len(got) != len(want) {
			t.Fatalf("%s: %d rows in the table, want %d", name, len(got), len(want))
		}
		for i := range want {
			if !floats.Equal(got[i], want[i]) {
				t.Fatalf("%s row %d = %v, want %v", name, i, got[i], want[i])
			}
		}
	}
}
