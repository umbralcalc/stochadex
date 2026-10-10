//go:build duckdb_arrow

package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestStagedDuckDBOutput(t *testing.T) {
	count := func(t *testing.T, path string) int {
		t.Helper()
		db, err := sql.Open("duckdb", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var tables int
		if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables WHERE table_name = 'runs'`).
			Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if tables == 0 {
			return -1
		}
		var rows int
		if err := db.QueryRow(`SELECT count(*) FROM runs`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}

	t.Run("the run is ingested at Commit, not Finalize", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "runs.duckdb")
		sink := stagedSink(t, "duckdb", map[string]interface{}{"path": path, "table": "runs"})
		writeWalk(sink)
		if !notExists(path) && count(t, path) != -1 {
			t.Fatal("the run was ingested before Commit")
		}
		if err := sink.Commit(); err != nil {
			t.Fatal(err)
		}
		if rows := count(t, path); rows != 2 {
			t.Errorf("the table has %d rows, want the run's 2", rows)
		}
	})

	t.Run("Abort ingests nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "runs.duckdb")
		sink := stagedSink(t, "duckdb", map[string]interface{}{"path": path, "table": "runs"})
		writeWalk(sink)
		sink.Abort()
		if !notExists(path) && count(t, path) != -1 {
			t.Error("an aborted run was ingested")
		}
	})
}
